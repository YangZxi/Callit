package executor

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"callit/internal/model"
	"callit/internal/worker"
)

func TestBuildSandboxSpecIncludesWorkerRuntimeAndUploadMounts(t *testing.T) {
	workspaceDir := filepath.Join(t.TempDir(), "workers", "worker-1")
	runtimeDir := filepath.Join(t.TempDir(), ".lib")
	workerRunningTempDir := filepath.Join(t.TempDir(), "req-1")
	uploadDir := filepath.Join(workerRunningTempDir, "upload")
	if err := os.MkdirAll(workspaceDir, 0o755); err != nil {
		t.Fatalf("创建 Worker 目录失败: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(runtimeDir, "node"), 0o755); err != nil {
		t.Fatalf("创建 runtime 依赖目录失败: %v", err)
	}
	if err := os.MkdirAll(uploadDir, 0o755); err != nil {
		t.Fatalf("创建上传目录失败: %v", err)
	}

	spec, err := buildSandboxSpec(sandboxCommandInput{
		Parent: context.Background(),
		WorkerSpec: mustNewRuntimeWorkerSpec(t,
			filepath.Join(filepath.Dir(filepath.Dir(workspaceDir)), "workers"),
			filepath.Dir(workerRunningTempDir),
			runtimeDir,
			model.Worker{ID: "worker-1", Runtime: "node", TimeoutMS: 5000},
			filepath.Base(workerRunningTempDir),
		),
	})
	if err != nil {
		t.Fatalf("构建沙箱配置失败: %v", err)
	}

	assertMountExists := func(dst string, readOnly bool, fstype string) {
		t.Helper()
		for _, mount := range spec.Mounts {
			if mount.Destination == dst && mount.ReadOnly == readOnly && mount.FSType == fstype {
				return
			}
		}
		t.Fatalf("未找到挂载点 dst=%q readOnly=%v fstype=%q，实际=%#v", dst, readOnly, fstype, spec.Mounts)
	}

	if spec.CWD != "/workspace" {
		t.Fatalf("沙箱工作目录不正确，got=%q want=%q", spec.CWD, "/workspace")
	}
	assertMountExists("/workspace", true, "bind")
	assertMountExists("/data", false, "bind")
	assertMountExists("/runtime-lib", true, "bind")
	assertMountExists("/tmp", false, "bind")
}

func mustNewRuntimeWorkerSpec(t *testing.T, workersDir string, workerTempBaseDir string, runtimeLibDir string, workerModel model.Worker, requestID string) worker.WorkerSpec {
	t.Helper()
	spec, err := worker.NewRuntimeWorkerSpec(workersDir, workerTempBaseDir, runtimeLibDir, workerModel, requestID)
	if err != nil {
		t.Fatalf("构造 Runtime WorkerSpec 失败: %v", err)
	}
	return spec
}

func TestBuildNsJailArgsDoesNotIncludeCgroupPidsMax(t *testing.T) {
	args := buildNsJailArgs("/tmp/test.cfg", sandboxSpec{
		CommandPath:  "/usr/bin/python3",
		CommandArgs:  []string{"-c", "print(1)"},
		RlimitCPUSec: 5,
		RlimitNoFile: 64,
	}, 5)

	if slices.Contains(args, "--cgroup_pids_max") {
		t.Fatalf("不应再包含 --cgroup_pids_max，args=%v", args)
	}
	if slices.Contains(args, "--rlimit_as") {
		t.Fatalf("不应再包含 --rlimit_as，args=%v", args)
	}
}

func TestBuildNsJailArgsIncludesKeepEnv(t *testing.T) {
	args := buildNsJailArgs("/tmp/test.cfg", sandboxSpec{
		CommandPath:  "/usr/bin/node",
		CommandArgs:  []string{"-e", "console.log(process.env.NODE_PATH)"},
		RlimitCPUSec: 5,
		RlimitNoFile: 64,
	}, 5)

	if !slices.Contains(args, "--keep_env") {
		t.Fatalf("应包含 --keep_env，避免沙箱内丢失 NODE_PATH/PYTHONPATH，args=%v", args)
	}
}

func TestBuildSandboxEnvForNode(t *testing.T) {
	runtimeDir := filepath.Join("/data", ".lib")
	executablePath := "/opt/node/bin/node"

	envList := buildSandboxEnv(runtimeDir, "node", executablePath, workerEnvConfig{})

	var nodePath string
	for _, item := range envList {
		if strings.HasPrefix(item, "NODE_PATH=") {
			nodePath = strings.TrimPrefix(item, "NODE_PATH=")
			break
		}
	}
	if nodePath == "" {
		t.Fatalf("应设置 NODE_PATH，env=%v", envList)
	}

	want := strings.Join([]string{
		"/runtime-lib/node_modules",
		"/opt/node/lib/node_modules",
		"/usr/local/lib/node_modules",
		"/usr/lib/node_modules",
	}, string(os.PathListSeparator))
	if nodePath != want {
		t.Fatalf("NODE_PATH 不正确，got=%q want=%q", nodePath, want)
	}
}

func TestBuildSandboxEnvForPython(t *testing.T) {
	runtimeDir := t.TempDir()
	sitePackages := filepath.Join(runtimeDir, "python", "venv", "lib", "python3.10", "site-packages")
	if err := os.MkdirAll(sitePackages, 0o755); err != nil {
		t.Fatalf("创建 site-packages 失败: %v", err)
	}

	envList := buildSandboxEnv(runtimeDir, "python", "/usr/bin/python3", workerEnvConfig{})

	var pythonPath string
	for _, item := range envList {
		if strings.HasPrefix(item, "PYTHONPATH=") {
			pythonPath = strings.TrimPrefix(item, "PYTHONPATH=")
			break
		}
	}
	if pythonPath != strings.Join([]string{"/runtime-lib", "/runtime-lib/venv/lib/python3.10/site-packages"}, string(os.PathListSeparator)) {
		t.Fatalf("PYTHONPATH 不正确，got=%q", pythonPath)
	}
}

func TestBuildSandboxEnvForPythonUsesSdkOutsideVenv(t *testing.T) {
	runtimeDir := t.TempDir()
	sitePackages := filepath.Join(runtimeDir, "python", "venv", "lib", "python3.10", "site-packages")
	if err := os.MkdirAll(sitePackages, 0o755); err != nil {
		t.Fatalf("创建 site-packages 失败: %v", err)
	}

	envList := buildSandboxEnv(runtimeDir, "python", "/usr/bin/python3", workerEnvConfig{})

	var pythonPath string
	for _, item := range envList {
		if strings.HasPrefix(item, "PYTHONPATH=") {
			pythonPath = strings.TrimPrefix(item, "PYTHONPATH=")
			break
		}
	}
	if pythonPath != strings.Join([]string{"/runtime-lib", "/runtime-lib/venv/lib/python3.10/site-packages"}, string(os.PathListSeparator)) {
		t.Fatalf("PYTHONPATH 不正确，got=%q", pythonPath)
	}
}

func TestBuildSandboxEnvIncludesKVEnv(t *testing.T) {
	envList := buildSandboxEnv("/data/.lib", "node", "/usr/bin/node", workerEnvConfig{
		CallitMagicApiBaseURL: "http://127.0.0.1:31001",
		WorkerID:              "worker-1",
		RequestID:             "req-1",
	})

	assertContains := func(want string) {
		t.Helper()
		if !slices.Contains(envList, want) {
			t.Fatalf("环境变量中缺少 %q，env=%v", want, envList)
		}
	}

	assertContains("CALLIT_MAGIC_API_BASE_URL=http://127.0.0.1:31001")
	assertContains("CALLIT_WORKER_ID=worker-1")
	assertContains("CALLIT_REQUEST_ID=req-1")
	if slices.ContainsFunc(envList, func(item string) bool {
		return strings.HasPrefix(item, "CALLIT_INTERNAL_TOKEN=")
	}) {
		t.Fatalf("环境变量中不应包含 CALLIT_INTERNAL_TOKEN，env=%v", envList)
	}

	var pathValue string
	for _, item := range envList {
		if strings.HasPrefix(item, "PATH=") {
			pathValue = strings.TrimPrefix(item, "PATH=")
			break
		}
	}
	if pathValue == "" {
		t.Fatalf("PATH 不应为空")
	}
	if strings.Contains(pathValue, "/callit-bin") {
		t.Fatalf("PATH 中不应暴露 /callit-bin，got=%q", pathValue)
	}
}

func TestBuildSandboxEnvIncludesWorkerCustomEnv(t *testing.T) {
	envList := buildSandboxEnv("/data/.lib", "node", "/usr/bin/node", workerEnvConfig{
		CustomKV: map[string]string{
			"API_KEY": "test-key",
			"REGION":  "us",
		},
	})

	assertContains := func(want string) {
		t.Helper()
		if !slices.Contains(envList, want) {
			t.Fatalf("环境变量中缺少 %q，env=%v", want, envList)
		}
	}
	assertContains("API_KEY=test-key")
	assertContains("REGION=us")
}

func TestParseWorkerEnvPairs(t *testing.T) {
	parsed := parseWorkerEnvPairs(" API_KEY = test ;\nREGION=us \nINVALID\n=empty;A=B=C")
	if len(parsed) != 3 {
		t.Fatalf("解析结果数量不正确: %#v", parsed)
	}
	if parsed["API_KEY"] != "test" {
		t.Fatalf("API_KEY 解析失败: %#v", parsed)
	}
	if parsed["REGION"] != "us" {
		t.Fatalf("REGION 解析失败: %#v", parsed)
	}
	if parsed["A"] != "B=C" {
		t.Fatalf("A 解析失败: %#v", parsed)
	}
}

func TestRuntimeLibNameByRuntimeForPythonUsesFixedVersionDir(t *testing.T) {
	if got := runtimeLibNameByRuntime("python"); got != "python" {
		t.Fatalf("Python runtime 目录不正确，got=%q", got)
	}
}

func TestRenderSandboxConfigIncludesCgroupV2MemoryConfig(t *testing.T) {
	cfg, err := renderSandboxConfig(sandboxSpec{
		CWD:               "/workspace",
		CommandPath:       "/usr/local/bin/node",
		CommandArgs:       []string{"-e", "console.log(1)"},
		RlimitCPUSec:      5,
		RlimitNoFile:      64,
		CgroupMemMaxBytes: 512 * 1024 * 1024,
		EnableCgroupV2:    true,
		Mounts:            []sandboxMount{{Destination: "/proc", FSType: "proc"}},
	})
	if err != nil {
		t.Fatalf("渲染 nsjail 配置失败: %v", err)
	}

	mustContain := []string{
		"detect_cgroupv2: true",
		"use_cgroupv2: true",
		"cgroupv2_mount: \"/sys/fs/cgroup\"",
		"cgroup_mem_max: 536870912",
	}
	for _, part := range mustContain {
		if !strings.Contains(cfg, part) {
			t.Fatalf("配置中缺少 %q，实际配置:\n%s", part, cfg)
		}
	}
}

func TestRuntimeSupportMountPathsIncludesSystemNetworkFiles(t *testing.T) {
	paths := runtimeSupportMountPaths()
	if !slices.Contains(paths, "/usr") {
		t.Fatalf("运行时应挂载整个 /usr 目录，paths=%v", paths)
	}
	if !slices.Contains(paths, "/etc/resolv.conf") {
		t.Fatalf("运行时应挂载 /etc/resolv.conf，paths=%v", paths)
	}
}

func TestBuildScriptExecuteErrorUsesReadonlyMessage(t *testing.T) {
	err := buildScriptExecuteError(
		"Traceback: [Errno 30] Read-only file system: './test.txt'",
		errors.New("exit status 1"),
	)
	if err == nil {
		t.Fatalf("应返回错误")
	}
	if err.Error() != "不允许在只读文件中执行写入操作" {
		t.Fatalf("错误文案不正确，got=%q", err.Error())
	}
}

func TestSplitBridgeOutputExtractsResultBlockBeforeTrailingNsJailLogs(t *testing.T) {
	raw := strings.Join([]string{
		"[I][2026-03-27T03:35:20+0000] Mode: STANDALONE_ONCE",
		"===============",
		"worker normal log",
		"**=====^=====**",
		`{"status":200,"body":"ok"}`,
		"**=====^=====**",
		"[I][2026-03-27T03:35:20+0000] pid=17 ([STANDALONE MODE]) exited with status: 0, (PIDs left: 0)",
	}, "\n")

	logOutput, resultOutput, err := splitBridgeOutput(raw)
	if err != nil {
		t.Fatalf("splitBridgeOutput 失败: %v", err)
	}

	wantLogOutput := strings.Join([]string{
		"[I][2026-03-27T03:35:20+0000] Mode: STANDALONE_ONCE",
		"===============",
		"worker normal log",
		"[I][2026-03-27T03:35:20+0000] pid=17 ([STANDALONE MODE]) exited with status: 0, (PIDs left: 0)",
	}, "\n")
	if logOutput != wantLogOutput {
		t.Fatalf("日志输出不正确，got=%q want=%q", logOutput, wantLogOutput)
	}
	if resultOutput != `{"status":200,"body":"ok"}` {
		t.Fatalf("结果输出不正确，got=%q", resultOutput)
	}
}

func TestNodeWorkerEntrypointKeepsAsyncLogsBeforeResultBlock(t *testing.T) {
	workerDir := t.TempDir()
	currentDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("获取当前目录失败: %v", err)
	}
	mainJS := strings.TrimSpace(`
async function handler() {
  await Promise.resolve();
  console.log({ rows: [], rows_affected: 0, last_insert_id: 0 });
  return { status: 200, body: { ok: true } };
}

module.exports = { handler };
`)
	if err := os.WriteFile(filepath.Join(workerDir, "main.js"), []byte(mainJS), 0o644); err != nil {
		t.Fatalf("写入 main.js 失败: %v", err)
	}

	cmd := exec.Command("node", filepath.Join(currentDir, "worker_entrypoints", "node.js"))
	cmd.Dir = workerDir
	cmd.Env = append(os.Environ(), "NODE_PATH="+filepath.Join(currentDir, "..", "..", "..", "data", ".lib", "node", "node_modules"))
	cmd.Stdin = strings.NewReader(`{"request":{"method":"GET","uri":"/","url":"http://127.0.0.1/"}}`)

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("执行 node worker entrypoint 失败: %v, output=%s", err, string(output))
	}

	logOutput, resultOutput, err := splitBridgeOutput(string(output))
	if err != nil {
		t.Fatalf("splitBridgeOutput 失败: %v, output=%s", err, string(output))
	}

	wantRawSequence := strings.Join([]string{
		"===============",
		"{ rows: [], rows_affected: 0, last_insert_id: 0 }",
		"",
		"**=====^=====**",
		`{"status":200,"body":{"ok":true}}`,
		"**=====^=====**",
		"===============",
	}, "\n")
	if !strings.Contains(strings.TrimSpace(string(output)), wantRawSequence) {
		t.Fatalf("原始输出顺序不正确，got=%q want包含=%q", strings.TrimSpace(string(output)), wantRawSequence)
	}

	wantLogOutput := strings.Join([]string{
		"===============",
		"{ rows: [], rows_affected: 0, last_insert_id: 0 }",
		"===============",
	}, "\n")
	if strings.TrimSpace(logOutput) != wantLogOutput {
		t.Fatalf("异步日志位置不正确，got=%q want=%q", strings.TrimSpace(logOutput), wantLogOutput)
	}
	if resultOutput != `{"status":200,"body":{"ok":true}}` {
		t.Fatalf("结果输出不正确，got=%q", resultOutput)
	}
}

func TestNodeWorkerEntrypointKeepsErrorInsideLogSeparator(t *testing.T) {
	workerDir := t.TempDir()
	currentDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("获取当前目录失败: %v", err)
	}
	mainJS := strings.TrimSpace(`
async function handler() {
  await Promise.resolve();
  throw new Error("boom");
}

module.exports = { handler };
`)
	if err := os.WriteFile(filepath.Join(workerDir, "main.js"), []byte(mainJS), 0o644); err != nil {
		t.Fatalf("写入 main.js 失败: %v", err)
	}

	cmd := exec.Command("node", filepath.Join(currentDir, "worker_entrypoints", "node.js"))
	cmd.Dir = workerDir
	cmd.Env = append(os.Environ(), "NODE_PATH="+filepath.Join(currentDir, "..", "..", "..", "data", ".lib", "node", "node_modules"))
	cmd.Stdin = strings.NewReader(`{"request":{"method":"GET","uri":"/","url":"http://127.0.0.1/"}}`)

	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("执行 node worker entrypoint 应失败，output=%s", string(output))
	}

	rawOutput := strings.TrimSpace(string(output))
	startIdx := strings.Index(rawOutput, "===============")
	endIdx := strings.LastIndex(rawOutput, "===============")
	if startIdx < 0 || endIdx <= startIdx {
		t.Fatalf("错误输出缺少外层日志分隔符，output=%q", rawOutput)
	}
	if !strings.Contains(rawOutput[startIdx:endIdx], "Error: boom") {
		t.Fatalf("错误日志未落在日志分隔符内，output=%q", rawOutput)
	}
}
