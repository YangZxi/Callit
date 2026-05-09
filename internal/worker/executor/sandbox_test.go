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

func TestBuildSandboxSpecForNodeESMIncludesWorkspaceNodeModulesMount(t *testing.T) {
	workerRootDir := filepath.Join(t.TempDir(), "workers", "worker-esm")
	workspaceDir := filepath.Join(workerRootDir, "code")
	runtimeDir := filepath.Join(t.TempDir(), ".lib")
	workerRunningTempDir := filepath.Join(t.TempDir(), "req-esm")
	if err := os.MkdirAll(filepath.Join(workspaceDir), 0o755); err != nil {
		t.Fatalf("创建 Worker 目录失败: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspaceDir, "package.json"), []byte("{\n  \"type\": \"module\"\n}\n"), 0o644); err != nil {
		t.Fatalf("写入 package.json 失败: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(runtimeDir, "node", "node_modules"), 0o755); err != nil {
		t.Fatalf("创建 runtime 依赖目录失败: %v", err)
	}
	if err := os.MkdirAll(workerRunningTempDir, 0o755); err != nil {
		t.Fatalf("创建运行时目录失败: %v", err)
	}

	spec, err := buildSandboxSpec(sandboxCommandInput{
		Parent: context.Background(),
		WorkerSpec: mustNewRuntimeWorkerSpec(t,
			filepath.Join(filepath.Dir(workerRootDir)),
			filepath.Dir(workerRunningTempDir),
			runtimeDir,
			model.Worker{ID: "worker-esm", Runtime: "node", TimeoutMS: 5000},
			filepath.Base(workerRunningTempDir),
		),
	})
	if err != nil {
		t.Fatalf("构建沙箱配置失败: %v", err)
	}

	for _, mount := range spec.Mounts {
		if mount.Destination == "/workspace/node_modules" && mount.Source == filepath.Join(runtimeDir, "node", "node_modules") {
			return
		}
	}
	t.Fatalf("ESM 模式应挂载 /workspace/node_modules，实际=%#v", spec.Mounts)
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

func TestBuildSandboxEnvForNodeESMDoesNotSetNodePath(t *testing.T) {
	envList := buildSandboxEnv("/data/.lib", "node", "/usr/bin/node", workerEnvConfig{
		NodeModuleType: nodeModuleTypeESM,
	})

	for _, item := range envList {
		if strings.HasPrefix(item, "NODE_PATH=") {
			t.Fatalf("ESM 模式不应设置 NODE_PATH，env=%v", envList)
		}
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
		CustomEnv: []string{"API_KEY=test-key", "REGION=us"},
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

func TestBuildSandboxWorkerEnv(t *testing.T) {
	parsed := buildSandboxWorkerEnv(model.WorkerEnv{" API_KEY = test ", "REGION=us ", "INVALID", "=empty", "A=B=C", "COOKIE=foo=bar;hello=world"})
	if len(parsed) != 4 {
		t.Fatalf("解析结果数量不正确: %#v", parsed)
	}
	if parsed[0] != "API_KEY=test" {
		t.Fatalf("API_KEY 解析失败: %#v", parsed)
	}
	if parsed[1] != "REGION=us" {
		t.Fatalf("REGION 解析失败: %#v", parsed)
	}
	if parsed[2] != "A=B=C" {
		t.Fatalf("A 解析失败: %#v", parsed)
	}
	if parsed[3] != "COOKIE=foo=bar;hello=world" {
		t.Fatalf("COOKIE 解析失败: %#v", parsed)
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

	cmd := exec.Command("node", filepath.Join(currentDir, "worker_entrypoints", "node_cjs.js"))
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

	cmd := exec.Command("node", filepath.Join(currentDir, "worker_entrypoints", "node_cjs.js"))
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

func TestDetectNodeModuleType(t *testing.T) {
	workerDir := t.TempDir()

	got, err := detectNodeModuleType(workerDir)
	if err != nil {
		t.Fatalf("未配置 package.json 时不应报错: %v", err)
	}
	if got != nodeModuleTypeCommonJS {
		t.Fatalf("默认应为 CommonJS，got=%q", got)
	}

	if err := os.WriteFile(filepath.Join(workerDir, "package.json"), []byte("{\n  \"type\": \"module\"\n}\n"), 0o644); err != nil {
		t.Fatalf("写入 package.json 失败: %v", err)
	}
	got, err = detectNodeModuleType(workerDir)
	if err != nil {
		t.Fatalf("合法 package.json 不应报错: %v", err)
	}
	if got != nodeModuleTypeESM {
		t.Fatalf("type=module 应识别为 ESM，got=%q", got)
	}
}

func TestDetectNodeModuleTypeRejectsInvalidPackageJSON(t *testing.T) {
	workerDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(workerDir, "package.json"), []byte("{"), 0o644); err != nil {
		t.Fatalf("写入 package.json 失败: %v", err)
	}

	_, err := detectNodeModuleType(workerDir)
	if err == nil {
		t.Fatalf("非法 package.json 应返回错误")
	}
	if !strings.Contains(err.Error(), "package.json") {
		t.Fatalf("错误信息应包含 package.json，got=%v", err)
	}
}

func TestNodeESMEntrypointSupportsDefaultExportAndImports(t *testing.T) {
	workerDir := t.TempDir()
	currentDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("获取当前目录失败: %v", err)
	}

	if err := os.WriteFile(filepath.Join(workerDir, "package.json"), []byte("{\n  \"type\": \"module\"\n}\n"), 0o644); err != nil {
		t.Fatalf("写入 package.json 失败: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(workerDir, "lib"), 0o755); err != nil {
		t.Fatalf("创建 lib 目录失败: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workerDir, "lib", "helper.js"), []byte("export function pickMessage() { return 'esm-ok'; }\n"), 0o644); err != nil {
		t.Fatalf("写入 helper.js 失败: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(workerDir, "node_modules", "axios"), 0o755); err != nil {
		t.Fatalf("创建 axios 目录失败: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workerDir, "node_modules", "axios", "package.json"), []byte("{\n  \"name\": \"axios\",\n  \"type\": \"module\",\n  \"exports\": \"./index.js\"\n}\n"), 0o644); err != nil {
		t.Fatalf("写入 axios package.json 失败: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workerDir, "node_modules", "axios", "index.js"), []byte("export default { name: 'axios-stub' };\n"), 0o644); err != nil {
		t.Fatalf("写入 axios index.js 失败: %v", err)
	}
	if err := os.Symlink(filepath.Join(currentDir, "..", "..", "..", "resources", "worker_sdk", "node", "callit"), filepath.Join(workerDir, "node_modules", "callit")); err != nil {
		t.Fatalf("创建 callit SDK 链接失败: %v", err)
	}

	mainJS := strings.TrimSpace(`
import axios from "axios";
import { kv, db } from "callit";
import { pickMessage } from "./lib/helper.js";

export default async function handler(ctx) {
  return {
    status: 200,
    body: {
      message: pickMessage(),
      axiosName: axios.name,
      hasKV: typeof kv?.newClient === "function",
      hasDB: typeof db?.newClient === "function",
      method: ctx.request.method
    }
  };
}
`)
	if err := os.WriteFile(filepath.Join(workerDir, "main.js"), []byte(mainJS), 0o644); err != nil {
		t.Fatalf("写入 main.js 失败: %v", err)
	}

	cmd := exec.Command("node", filepath.Join(currentDir, "worker_entrypoints", "node_esm.js"))
	cmd.Dir = workerDir
	cmd.Stdin = strings.NewReader(`{"request":{"method":"GET","uri":"/","url":"http://127.0.0.1/"}}`)

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("执行 node esm worker entrypoint 失败: %v, output=%s", err, string(output))
	}

	_, resultOutput, err := splitBridgeOutput(string(output))
	if err != nil {
		t.Fatalf("splitBridgeOutput 失败: %v, output=%s", err, string(output))
	}

	wantParts := []string{
		`"message":"esm-ok"`,
		`"axiosName":"axios-stub"`,
		`"hasKV":true`,
		`"hasDB":true`,
		`"method":"GET"`,
	}
	for _, part := range wantParts {
		if !strings.Contains(resultOutput, part) {
			t.Fatalf("ESM 结果缺少 %q，got=%s", part, resultOutput)
		}
	}
}

func TestNodeESMEntrypointSupportsNamedHandlerExport(t *testing.T) {
	workerDir := t.TempDir()
	currentDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("获取当前目录失败: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workerDir, "package.json"), []byte("{\n  \"type\": \"module\"\n}\n"), 0o644); err != nil {
		t.Fatalf("写入 package.json 失败: %v", err)
	}
	mainJS := "export function handler(ctx) { return { status: 200, body: { uri: ctx.request.uri } }; }\n"
	if err := os.WriteFile(filepath.Join(workerDir, "main.js"), []byte(mainJS), 0o644); err != nil {
		t.Fatalf("写入 main.js 失败: %v", err)
	}

	cmd := exec.Command("node", filepath.Join(currentDir, "worker_entrypoints", "node_esm.js"))
	cmd.Dir = workerDir
	cmd.Stdin = strings.NewReader(`{"request":{"method":"GET","uri":"/named","url":"http://127.0.0.1/named"}}`)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("执行 node esm worker entrypoint 失败: %v, output=%s", err, string(output))
	}

	_, resultOutput, err := splitBridgeOutput(string(output))
	if err != nil {
		t.Fatalf("splitBridgeOutput 失败: %v, output=%s", err, string(output))
	}
	if !strings.Contains(resultOutput, `"uri":"/named"`) {
		t.Fatalf("命名导出结果不正确，got=%s", resultOutput)
	}
}
