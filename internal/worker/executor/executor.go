package executor

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"callit/internal/config"
	"callit/internal/model"
	"callit/internal/worker"
)

//go:embed worker_entrypoints/*
var workerEntrypointsFS embed.FS

const logAndOutputSeparator = "\n**=====^=====**\n"
const workerLogSeparatorLine = "==============="
const (
	defaultPythonCgroupMemMaxBytes = 64 * 1024 * 1024
	defaultNodeCgroupMemMaxBytes   = 64 * 1024 * 1024
	defaultSandboxRlimitFile       = 128
	fixedPythonCommand             = "python3.10"

	sandboxWorkspacePath           = "/workspace"
	sandboxWorkspaceNodeModulesDir = "/workspace/node_modules"
	sandboxRuntimePath             = "/runtime-lib"
	sandboxTmpPath                 = "/tmp"
	sandboxDataPath                = "/data"
)

type nodeModuleType string

const (
	nodeModuleTypeCommonJS nodeModuleType = "commonjs"
	nodeModuleTypeESM      nodeModuleType = "module"
)

// ExecuteResult 表示脚本执行结果。
type ExecuteResult struct {
	Status     int
	Headers    map[string]string
	File       string
	Body       any
	Stdout     string
	Stderr     string
	ExecLog    string
	Result     string
	DurationMS int64
	TimedOut   bool
	Err        error
}

// Run 执行函数脚本。
func Run(parent context.Context, workerSpec worker.WorkerSpec, cfg config.Config, input model.WorkerInput) (result ExecuteResult) {
	started := time.Now()
	result = ExecuteResult{Headers: map[string]string{}}
	defer func() {
		result.DurationMS = time.Since(started).Milliseconds()
	}()

	worker := workerSpec.Worker
	mainFile := workerSpec.MainFilename()
	if mainFile == "" {
		result.Err = fmt.Errorf("不支持的 runtime: %s", worker.Runtime)
		slog.Error("不支持的 runtime", "runtime", worker.Runtime)
		return
	}
	if _, err := os.Stat(filepath.Join(workerSpec.WorkerCodeDir, mainFile)); err != nil {
		result.Err = fmt.Errorf("主文件不存在: %s", mainFile)
		slog.Error("Worker 主文件不存在", "worker_dir", workerSpec.WorkerCodeDir, "main_file", mainFile, "err", err)
		return
	}

	payload, err := json.Marshal(input)
	if err != nil {
		result.Err = fmt.Errorf("序列化脚本上下文失败: %w", err)
		slog.Error("序列化脚本上下文失败", "worker_id", worker.ID, "request_id", input.Event.RequestID, "err", err)
		return
	}

	bridgeStdout, bridgeStderr, runErr := runWithNsJailBridge(sandboxCommandInput{
		Parent:         parent,
		WorkerSpec:     workerSpec,
		RequestID:      input.Event.RequestID,
		EnableCgroupV2: cfg.EnableCgroupV2,
		ServerPort:     cfg.MagicServerPort,
		Payload:        payload,
	})
	result.ExecLog = bridgeStdout
	result.Stderr = bridgeStderr

	if errors.Is(parent.Err(), context.DeadlineExceeded) {
		result.TimedOut = true
		result.Err = context.DeadlineExceeded
		slog.Warn("Worker 执行超时", "worker_id", worker.ID, "request_id", input.Event.RequestID)
		return
	}
	if runErr != nil {
		// slog.Error("脚本执行失败", "worker_id", worker.ID, "request_id", input.Event.RequestID, "stderr", bridgeStderr, "err", runErr)
		result.Stdout = extractWorkerStdout(result.ExecLog)
		result.Err = buildScriptExecuteError(bridgeStderr, runErr)
		return
	}

	logOutput, resultOutput, err := splitBridgeOutput(bridgeStdout)
	if err != nil {
		result.Stdout = extractWorkerStdout(result.ExecLog)
		result.Err = fmt.Errorf("脚本 stdout 格式错误: %w", err)
		return
	}
	_ = logOutput
	result.Stdout = extractWorkerStdout(result.ExecLog)
	result.Result = resultOutput

	scriptOut, err := parseScriptOutput([]byte(resultOutput))
	if err != nil {
		result.Err = fmt.Errorf("脚本输出结果不是合法 JSON: %w", err)
		return
	}

	status := 200
	if scriptOut.Status != nil {
		status = *scriptOut.Status
	}
	if status < 100 || status > 599 {
		result.Err = fmt.Errorf("脚本返回非法 status: %d", status)
		return
	}
	result.Status = status
	if scriptOut.Headers != nil {
		result.Headers = scriptOut.Headers
	}
	result.File = scriptOut.File
	result.Body = scriptOut.Body
	return
}

func mainFilenameByRuntime(runtime string) string {
	switch runtime {
	case "python":
		return "main.py"
	case "node":
		return "main.js"
	default:
		return ""
	}
}

func parseScriptOutput(raw []byte) (model.WorkerOutput, error) {
	var scriptOut model.WorkerOutput
	if err := json.Unmarshal(raw, &scriptOut); err != nil {
		return model.WorkerOutput{}, err
	}
	return scriptOut, nil
}

type sandboxCommandInput struct {
	Parent         context.Context
	WorkerSpec     worker.WorkerSpec
	RequestID      string
	EnableCgroupV2 bool
	ServerPort     int
	Payload        []byte
}

func runWithNsJailBridge(input sandboxCommandInput) (stdout string, stderr string, err error) {
	bridgeCmd, cleanup, err := buildSandboxCommand(input)
	if err != nil {
		return "", "", err
	}
	defer cleanup()
	bridgeCmd.Stdin = bytes.NewReader(input.Payload)

	var out, errOut bytes.Buffer
	bridgeCmd.Stdout = &out
	bridgeCmd.Stderr = &errOut

	if err := bridgeCmd.Run(); err != nil {
		return out.String(), errOut.String(), err
	}
	return out.String(), errOut.String(), nil
}

type sandboxSpec struct {
	CWD               string
	CommandPath       string
	CommandArgs       []string
	Mounts            []sandboxMount
	CgroupMemMaxBytes int64
	EnableCgroupV2    bool
	RlimitCPUSec      int
	RlimitNoFile      int
}

type sandboxMount struct {
	Source      string
	Destination string
	ReadOnly    bool
	FSType      string
	IsBind      bool
}

func buildSandboxSpec(input sandboxCommandInput) (sandboxSpec, error) {
	workerCodeDir, err := filepath.Abs(input.WorkerSpec.WorkerCodeDir)
	if err != nil {
		return sandboxSpec{}, fmt.Errorf("解析 Worker 目录失败: %w", err)
	}
	workerDataDir, err := filepath.Abs(input.WorkerSpec.WorkerDataDir)
	if err != nil {
		return sandboxSpec{}, fmt.Errorf("解析 Worker data 目录失败: %w", err)
	}
	workerTmpDir, err := filepath.Abs(input.WorkerSpec.WorkerTempDir)
	if err != nil {
		return sandboxSpec{}, fmt.Errorf("解析运行时目录失败: %w", err)
	}
	runtimeDir, err := filepath.Abs(input.WorkerSpec.RuntimeLibDir)
	if err != nil {
		return sandboxSpec{}, fmt.Errorf("解析 runtime 依赖目录失败: %w", err)
	}
	runtimePath := filepath.Join(runtimeDir, runtimeLibNameByRuntime(input.WorkerSpec.Worker.Runtime))
	if info, statErr := os.Stat(runtimePath); statErr != nil || !info.IsDir() {
		if statErr == nil {
			statErr = fmt.Errorf("不是目录")
		}
		return sandboxSpec{}, fmt.Errorf("runtime 依赖目录不存在: %s: %w", runtimePath, statErr)
	}
	commandPath, err := resolveRuntimeExecutable(input.WorkerSpec.Worker.Runtime)
	if err != nil {
		return sandboxSpec{}, err
	}

	switch input.WorkerSpec.Worker.Runtime {
	case "python":
		return buildPythonSandboxSpec(input, commandPath, workerCodeDir, workerDataDir, runtimePath, workerTmpDir)
	case "node":
		return buildNodeSandboxSpec(input, commandPath, workerCodeDir, workerDataDir, runtimePath, workerTmpDir)
	default:
		return sandboxSpec{}, fmt.Errorf("不支持的 runtime: %s", input.WorkerSpec.Worker.Runtime)
	}
}

func buildPythonSandboxSpec(input sandboxCommandInput, commandPath string, workerCodeDir string, workerDataDir string, runtimePath string, workerTmpDir string) (sandboxSpec, error) {
	bridgeScript, err := readBridgeScriptByFilename("python.py")
	if err != nil {
		return sandboxSpec{}, err
	}
	spec := baseSandboxSpec(input, commandPath, workerCodeDir, workerDataDir, runtimePath, workerTmpDir)
	spec.CommandArgs = []string{"-c", bridgeScript}
	return spec, nil
}

func buildNodeSandboxSpec(input sandboxCommandInput, commandPath string, workerCodeDir string, workerDataDir string, runtimePath string, workerTmpDir string) (sandboxSpec, error) {
	moduleType, err := detectNodeModuleType(workerCodeDir)
	if err != nil {
		return sandboxSpec{}, err
	}
	switch moduleType {
	case nodeModuleTypeESM:
		return buildNodeSandboxSpecByESM(input, commandPath, workerCodeDir, workerDataDir, runtimePath, workerTmpDir)
	default:
		return buildNodeSandboxSpecByCommonJS(input, commandPath, workerCodeDir, workerDataDir, runtimePath, workerTmpDir)
	}
}

func buildNodeSandboxSpecByCommonJS(input sandboxCommandInput, commandPath string, workerCodeDir string, workerDataDir string, runtimePath string, workerTmpDir string) (sandboxSpec, error) {
	bridgeScript, err := readBridgeScriptByFilename("node_cjs.js")
	if err != nil {
		return sandboxSpec{}, err
	}
	spec := baseSandboxSpec(input, commandPath, workerCodeDir, workerDataDir, runtimePath, workerTmpDir)
	spec.CommandArgs = []string{"-e", bridgeScript}
	return spec, nil
}

func buildNodeSandboxSpecByESM(input sandboxCommandInput, commandPath string, workerCodeDir string, workerDataDir string, runtimePath string, workerTmpDir string) (sandboxSpec, error) {
	bridgeScript, err := readBridgeScriptByFilename("node_esm.js")
	if err != nil {
		return sandboxSpec{}, err
	}
	spec := baseSandboxSpec(input, commandPath, workerCodeDir, workerDataDir, runtimePath, workerTmpDir)
	spec.CommandArgs = []string{"-e", bridgeScript}
	spec.Mounts = append(spec.Mounts, sandboxMount{
		Source:      filepath.Join(runtimePath, "node_modules"),
		Destination: sandboxWorkspaceNodeModulesDir,
		ReadOnly:    true,
		IsBind:      true,
		FSType:      "bind",
	})
	return spec, nil
}

func baseSandboxSpec(input sandboxCommandInput, commandPath string, workerCodeDir string, workerDataDir string, runtimePath string, workerTmpDir string) sandboxSpec {
	spec := sandboxSpec{
		CWD:               sandboxWorkspacePath,
		CommandPath:       commandPath,
		CgroupMemMaxBytes: sandboxCgroupMemMaxBytesByRuntime(input.WorkerSpec.Worker.Runtime),
		EnableCgroupV2:    input.EnableCgroupV2,
		RlimitNoFile:      defaultSandboxRlimitFile,
	}
	spec.Mounts = append(spec.Mounts,
		sandboxMount{Source: workerCodeDir, Destination: sandboxWorkspacePath, ReadOnly: true, IsBind: true, FSType: "bind"},
		sandboxMount{Source: workerDataDir, Destination: sandboxDataPath, ReadOnly: false, IsBind: true, FSType: "bind"},
		sandboxMount{Source: runtimePath, Destination: sandboxRuntimePath, ReadOnly: true, IsBind: true, FSType: "bind"},
		sandboxMount{Source: workerTmpDir, Destination: sandboxTmpPath, ReadOnly: false, IsBind: true, FSType: "bind"},
		sandboxMount{Destination: "/proc", FSType: "proc"},
	)
	for _, mountPath := range runtimeSupportMountPaths() {
		if _, statErr := os.Stat(mountPath); statErr != nil {
			continue
		}
		spec.Mounts = append(spec.Mounts, sandboxMount{Source: mountPath, Destination: mountPath, ReadOnly: true, IsBind: true, FSType: "bind"})
	}
	slog.Debug("构建沙箱挂载配置", "worker_code_dir", workerCodeDir, "worker_data_dir", workerDataDir, "runtime_path", runtimePath, "temp_dir", input.WorkerSpec.WorkerTempDir, "mount_count", len(spec.Mounts))
	return spec
}

func sandboxCgroupMemMaxBytesByRuntime(runtime string) int64 {
	switch runtime {
	case "node":
		return defaultNodeCgroupMemMaxBytes
	default:
		return defaultPythonCgroupMemMaxBytes
	}
}

func runtimeLibNameByRuntime(runtime string) string {
	switch runtime {
	case "python":
		return "python"
	case "node":
		return "node"
	default:
		return runtime
	}
}

func resolveRuntimeExecutable(runtime string) (string, error) {
	switch runtime {
	case "python":
		return exec.LookPath(fixedPythonCommand)
	case "node":
		return exec.LookPath("node")
	default:
		return "", fmt.Errorf("不支持的 runtime: %s", runtime)
	}
}

func runtimeSupportMountPaths() []string {
	paths := []string{
		"/lib",
		"/lib64",
		"/usr",
		"/etc/resolv.conf",
		"/etc/hosts",
		"/etc/nsswitch.conf",
		"/etc/gai.conf",
		"/etc/ssl/certs",
		"/etc/ca-certificates.conf",
		"/etc/ssl/openssl.cnf",
	}
	return paths
}

func buildSandboxCommand(input sandboxCommandInput) (*exec.Cmd, func(), error) {
	cleanup := func() {}
	var err error
	timeLimit := input.WorkerSpec.Worker.TimeoutMS / 1000
	if input.WorkerSpec.Worker.TimeoutMS%1000 != 0 {
		timeLimit++
	}
	if timeLimit <= 0 {
		timeLimit = 1
	}

	spec, err := buildSandboxSpec(input)
	if err != nil {
		return nil, cleanup, err
	}
	spec.RlimitCPUSec = timeLimit

	configContent, err := renderSandboxConfig(spec)
	if err != nil {
		return nil, cleanup, err
	}

	configFile, err := os.CreateTemp("", "callit-nsjail-*.cfg")
	if err != nil {
		return nil, cleanup, fmt.Errorf("创建 nsjail 配置文件失败: %w", err)
	}
	configPath := configFile.Name()
	cleanup = func() {
		_ = os.Remove(configPath)
	}
	if _, err := configFile.WriteString(configContent); err != nil {
		_ = configFile.Close()
		return nil, cleanup, fmt.Errorf("写入 nsjail 配置失败: %w", err)
	}
	if err := configFile.Close(); err != nil {
		return nil, cleanup, fmt.Errorf("关闭 nsjail 配置文件失败: %w", err)
	}

	nsjailPath := os.Getenv("NSJAIL_BIN")
	if strings.TrimSpace(nsjailPath) == "" {
		nsjailPath = "nsjail"
	}
	if _, err := exec.LookPath(nsjailPath); err != nil {
		return nil, cleanup, fmt.Errorf("nsjail 不可用: %w", err)
	}

	args := buildNsJailArgs(configPath, spec, timeLimit)

	runtimeDirAbs, err := filepath.Abs(input.WorkerSpec.RuntimeLibDir)
	if err != nil {
		return nil, cleanup, fmt.Errorf("解析 runtime 依赖目录失败: %w", err)
	}
	cmd := exec.CommandContext(input.Parent, nsjailPath, args...)
	cmd.Dir = input.WorkerSpec.WorkerCodeDir
	nodeType := nodeModuleTypeCommonJS
	if input.WorkerSpec.Worker.Runtime == "node" {
		nodeType, err = detectNodeModuleType(input.WorkerSpec.WorkerCodeDir)
		if err != nil {
			return nil, cleanup, err
		}
	}
	cmd.Env = buildSandboxEnv(runtimeDirAbs, input.WorkerSpec.Worker.Runtime, spec.CommandPath, workerEnvConfig{
		CallitMagicApiBaseURL: fmt.Sprintf("http://127.0.0.1:%d", input.ServerPort),
		WorkerID:              input.WorkerSpec.Worker.ID,
		RequestID:             input.RequestID,
		CustomKV:              parseWorkerEnvPairs(input.WorkerSpec.Worker.Env),
		NodeModuleType:        nodeType,
	})
	return cmd, cleanup, nil
}

func buildNsJailArgs(configPath string, spec sandboxSpec, timeLimit int) []string {
	args := []string{
		"--config", configPath,
		"--log_fd", "1",
		"--keep_env",
	}
	if !spec.EnableCgroupV2 {
		args = append(args, "--disable_clone_newcgroup")
	}
	args = append(args,
		"--rlimit_cpu", strconv.Itoa(spec.RlimitCPUSec),
		"--rlimit_nofile", strconv.Itoa(spec.RlimitNoFile),
		"--time_limit", strconv.Itoa(timeLimit),
		"--disable_rlimits",
		"--",
		spec.CommandPath,
	)
	args = append(args, spec.CommandArgs...)
	return args
}

func renderSandboxConfig(spec sandboxSpec) (string, error) {
	var builder strings.Builder
	builder.WriteString("name: \"callit-worker\"\n")
	builder.WriteString("mode: ONCE\n")
	builder.WriteString("hostname: \"callit\"\n")
	builder.WriteString("cwd: \"" + spec.CWD + "\"\n")
	builder.WriteString("max_cpus: 1\n")
	builder.WriteString("clone_newnet: false\n")
	builder.WriteString("clone_newuser: true\n")
	builder.WriteString("clone_newns: true\n")
	builder.WriteString("clone_newpid: true\n")
	builder.WriteString("clone_newipc: true\n")
	builder.WriteString("clone_newuts: true\n")
	if spec.EnableCgroupV2 {
		builder.WriteString("detect_cgroupv2: true\n")
		builder.WriteString("use_cgroupv2: true\n")
		builder.WriteString("cgroupv2_mount: \"/sys/fs/cgroup\"\n")
		builder.WriteString("cgroup_mem_max: " + strconv.FormatInt(spec.CgroupMemMaxBytes, 10) + "\n")
	}
	builder.WriteString("uidmap {\n  inside_id: \"0\"\n  outside_id: \"\"\n  count: 1\n}\n")
	builder.WriteString("gidmap {\n  inside_id: \"0\"\n  outside_id: \"\"\n  count: 1\n}\n")
	for _, mount := range spec.Mounts {
		builder.WriteString("mount {\n")
		if mount.IsBind {
			builder.WriteString("  src: \"" + mount.Source + "\"\n")
			builder.WriteString("  dst: \"" + mount.Destination + "\"\n")
			builder.WriteString("  is_bind: true\n")
			if mount.ReadOnly {
				builder.WriteString("  rw: false\n")
			} else {
				builder.WriteString("  rw: true\n")
			}
		} else if mount.FSType == "tmpfs" {
			builder.WriteString("  dst: \"" + mount.Destination + "\"\n")
			builder.WriteString("  fstype: \"tmpfs\"\n")
			builder.WriteString("  rw: true\n")
		} else if mount.FSType == "proc" {
			builder.WriteString("  dst: \"" + mount.Destination + "\"\n")
			builder.WriteString("  fstype: \"proc\"\n")
		}
		builder.WriteString("}\n")
	}
	return builder.String(), nil
}

func readBridgeScript(runtime string) (string, error) {
	filename := ""
	switch runtime {
	case "python":
		filename = "python.py"
	case "node":
		filename = "node_cjs.js"
	default:
		return "", fmt.Errorf("不支持的 runtime: %s", runtime)
	}
	return readBridgeScriptByFilename(filename)
}

func readBridgeScriptByFilename(filename string) (string, error) {
	content, err := workerEntrypointsFS.ReadFile(filepath.ToSlash(filepath.Join("worker_entrypoints", filename)))
	if err != nil {
		return "", fmt.Errorf("读取入口脚本失败: %w", err)
	}
	return string(content), nil
}

func buildScriptExecuteError(stderr string, runErr error) error {
	if strings.Contains(stderr, "[Errno 30] Read-only file system") {
		slog.Warn("检测到只读目录写入", "stderr", stderr)
		return errors.New("不允许在只读文件中执行写入操作")
	}
	return fmt.Errorf("脚本执行失败: %w", runErr)
}

type workerEnvConfig struct {
	CallitMagicApiBaseURL string
	WorkerID              string
	RequestID             string
	CustomKV              map[string]string
	NodeModuleType        nodeModuleType
}

func buildSandboxEnv(runtimeDir string, runtime string, executablePath string, workerEnv workerEnvConfig) []string {
	envList := []string{
		"HOME=/tmp",
		"TMPDIR=/tmp",
		"LANG=C.UTF-8",
	}

	pathEntries := []string{}
	pathEntries = append(pathEntries, filepath.Dir(executablePath))
	pathEntries = append(pathEntries, "/usr/local/bin", "/usr/bin", "/bin")
	envList = append(envList, "PATH="+buildPathEntries(pathEntries))
	if workerEnv.CallitMagicApiBaseURL != "" {
		envList = append(envList,
			"CALLIT_MAGIC_API_BASE_URL="+workerEnv.CallitMagicApiBaseURL,
			"CALLIT_WORKER_ID="+workerEnv.WorkerID,
			"CALLIT_REQUEST_ID="+workerEnv.RequestID,
		)
	}
	if len(workerEnv.CustomKV) > 0 {
		keys := make([]string, 0, len(workerEnv.CustomKV))
		for key := range workerEnv.CustomKV {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			envList = append(envList, key+"="+workerEnv.CustomKV[key])
		}
	}

	switch runtime {
	case "node":
		if workerEnv.NodeModuleType == nodeModuleTypeESM {
			break
		}
		envList = appendRuntimeEnvPaths(envList, "NODE_PATH", nodeRuntimeModulePaths(runtimeDir, executablePath))
	case "python":
		envList = appendRuntimeEnvPaths(envList, "PYTHONPATH", pythonRuntimeModulePaths(runtimeDir))
	}
	return envList
}

func detectNodeModuleType(workerCodeDir string) (nodeModuleType, error) {
	packagePath := filepath.Join(workerCodeDir, "package.json")
	raw, err := os.ReadFile(packagePath)
	if errors.Is(err, os.ErrNotExist) {
		return nodeModuleTypeCommonJS, nil
	}
	if err != nil {
		return "", fmt.Errorf("读取 Worker package.json 失败: %w", err)
	}
	var pkg struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(raw, &pkg); err != nil {
		return "", fmt.Errorf("解析 Worker package.json 失败: %w", err)
	}
	if strings.EqualFold(strings.TrimSpace(pkg.Type), string(nodeModuleTypeESM)) {
		return nodeModuleTypeESM, nil
	}
	return nodeModuleTypeCommonJS, nil
}

func parseWorkerEnvPairs(envText string) map[string]string {
	entries := strings.FieldsFunc(envText, func(r rune) bool {
		return r == ';' || r == '\n'
	})
	envMap := make(map[string]string, len(entries))
	for _, item := range entries {
		raw := strings.TrimSpace(item)
		if raw == "" {
			continue
		}
		key, value, ok := strings.Cut(raw, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		envMap[key] = strings.TrimSpace(value)
	}
	return envMap
}

func appendRuntimeEnvPaths(env []string, key string, paths []string) []string {
	for _, item := range paths {
		env = appendOrMergeEnvPath(env, key, item)
	}
	return env
}

func nodeRuntimeModulePaths(runtimeDir string, executablePath string) []string {
	runtimeRoot := filepath.Join(runtimeDir, "node")
	paths := []string{
		mapRuntimePathToSandbox(filepath.Join(runtimeRoot, "node_modules"), runtimeRoot),
	}
	return append(paths, nodeGlobalModulePaths(executablePath)...)
}

func pythonRuntimeModulePaths(runtimeDir string) []string {
	runtimeRoot := filepath.Join(runtimeDir, runtimeLibNameByRuntime("python"))
	pythonEnvDir := filepath.Join(runtimeRoot, "venv")
	sitePackages := detectPythonSitePackages(pythonEnvDir)
	if sitePackages == "" {
		return []string{
			mapRuntimePathToSandbox(runtimeRoot, runtimeRoot),
		}
	}
	return []string{
		mapRuntimePathToSandbox(runtimeRoot, runtimeRoot),
		mapRuntimePathToSandbox(sitePackages, runtimeRoot),
	}
}

func mapRuntimePathToSandbox(hostPath string, runtimeRoot string) string {
	hostPath = filepath.Clean(strings.TrimSpace(hostPath))
	runtimeRoot = filepath.Clean(strings.TrimSpace(runtimeRoot))
	if hostPath == "" || runtimeRoot == "" {
		return hostPath
	}
	if hostPath == runtimeRoot {
		return sandboxRuntimePath
	}
	rel, err := filepath.Rel(runtimeRoot, hostPath)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		return hostPath
	}
	return filepath.Join(sandboxRuntimePath, rel)
}

func nodeGlobalModulePaths(executablePath string) []string {
	if strings.TrimSpace(executablePath) == "" {
		return nil
	}

	nodeBinDir := filepath.Dir(filepath.Clean(executablePath))
	nodeRootDir := filepath.Dir(nodeBinDir)
	return []string{
		filepath.Join(nodeRootDir, "lib", "node_modules"),
		"/usr/local/lib/node_modules",
		"/usr/lib/node_modules",
	}
}

// buildPathEntries 将路径切片去重后按系统分隔符拼成 PATH 值。
func buildPathEntries(entries []string) string {
	seen := make(map[string]struct{}, len(entries))
	ordered := make([]string, 0, len(entries))
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if _, ok := seen[entry]; ok {
			continue
		}
		seen[entry] = struct{}{}
		ordered = append(ordered, entry)
	}
	return strings.Join(ordered, string(os.PathListSeparator))
}

func splitBridgeOutput(stdout string) (logOutput string, resultOutput string, err error) {
	startIdx := strings.Index(stdout, logAndOutputSeparator)
	if startIdx < 0 {
		return "", "", fmt.Errorf("缺少结果分隔符")
	}
	resultStart := startIdx + len(logAndOutputSeparator)
	endRelIdx := strings.Index(stdout[resultStart:], logAndOutputSeparator)
	if endRelIdx < 0 {
		return "", "", fmt.Errorf("缺少结果结束分隔符")
	}
	endIdx := resultStart + endRelIdx

	logPrefix := stdout[:startIdx]
	logSuffix := stdout[endIdx+len(logAndOutputSeparator):]
	switch {
	case strings.TrimSpace(logPrefix) == "":
		logOutput = logSuffix
	case strings.TrimSpace(logSuffix) == "":
		logOutput = logPrefix
	default:
		if strings.HasSuffix(logPrefix, "\n") || strings.HasPrefix(logSuffix, "\n") {
			logOutput = logPrefix + logSuffix
		} else {
			logOutput = logPrefix + "\n" + logSuffix
		}
	}
	resultOutput = strings.TrimSpace(stdout[resultStart:endIdx])
	if resultOutput == "" {
		return "", "", fmt.Errorf("分隔符后结果为空")
	}
	return logOutput, resultOutput, nil
}

func extractWorkerStdout(execLog string) string {
	if strings.TrimSpace(execLog) == "" {
		return ""
	}

	logSource := execLog
	if logOutput, _, err := splitBridgeOutput(execLog); err == nil {
		logSource = logOutput
	}

	lines := strings.Split(logSource, "\n")
	startIdx := -1
	endIdx := -1
	for index, line := range lines {
		if strings.TrimSpace(line) != workerLogSeparatorLine {
			continue
		}
		if startIdx < 0 {
			startIdx = index
			continue
		}
		endIdx = index
	}

	if startIdx >= 0 && endIdx > startIdx {
		return strings.Trim(strings.Join(lines[startIdx+1:endIdx], "\n"), "\n")
	}

	return strings.TrimSpace(logSource)
}

// 用于在 Python 虚拟环境目录下定位真实的 site-packages 路径，供构建 PYTHONPATH 使用。
func detectPythonSitePackages(pythonEnvDir string) string {
	libDir := filepath.Join(pythonEnvDir, "lib")
	entries, err := os.ReadDir(libDir)
	if err != nil {
		return ""
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := strings.ToLower(strings.TrimSpace(entry.Name()))
		if !strings.HasPrefix(name, "python") {
			continue
		}
		sitePackages := filepath.Join(libDir, entry.Name(), "site-packages")
		stat, err := os.Stat(sitePackages)
		if err == nil && stat.IsDir() {
			return sitePackages
		}
	}
	return ""
}

func appendOrMergeEnvPath(env []string, key string, value string) []string {
	if strings.TrimSpace(value) == "" {
		return env
	}

	prefix := key + "="
	index := -1
	current := ""
	for i, item := range env {
		if strings.HasPrefix(item, prefix) {
			index = i
			current = strings.TrimPrefix(item, prefix)
			break
		}
	}

	parts := []string{}
	if strings.TrimSpace(current) != "" {
		parts = strings.Split(current, string(os.PathListSeparator))
	}
	for _, item := range parts {
		if item == value {
			return env
		}
	}

	var merged string
	if current == "" {
		merged = value
	} else {
		merged = current + string(os.PathListSeparator) + value
	}

	entry := prefix + merged
	if index >= 0 {
		env[index] = entry
		return env
	}
	return append(env, entry)
}
