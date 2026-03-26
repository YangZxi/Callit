package executor

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"callit/internal/model"
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
		Parent:               context.Background(),
		Worker:               model.Worker{Runtime: "node", TimeoutMS: 5000},
		WorkerDir:            workspaceDir,
		RuntimeDir:           runtimeDir,
		WorkerRunningTempDir: workerRunningTempDir,
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
	assertMountExists("/runtime-lib", true, "bind")
	assertMountExists("/tmp", false, "bind")
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

	envList := buildSandboxEnv(runtimeDir, "node", executablePath)

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
	sitePackages := filepath.Join(runtimeDir, "python", "lib", "python3.12", "site-packages")
	if err := os.MkdirAll(sitePackages, 0o755); err != nil {
		t.Fatalf("创建 site-packages 失败: %v", err)
	}

	envList := buildSandboxEnv(runtimeDir, "python", "/usr/bin/python3")

	var pythonPath string
	for _, item := range envList {
		if strings.HasPrefix(item, "PYTHONPATH=") {
			pythonPath = strings.TrimPrefix(item, "PYTHONPATH=")
			break
		}
	}
	if pythonPath != "/runtime-lib/lib/python3.12/site-packages" {
		t.Fatalf("PYTHONPATH 不正确，got=%q", pythonPath)
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
