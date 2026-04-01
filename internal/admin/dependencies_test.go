package admin

import (
	"context"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"callit/internal/config"
	"callit/internal/router"
)

func TestNormalizeDependencyRuntime(t *testing.T) {
	valid := []string{"node", "python", " Node ", "PYTHON"}
	for _, item := range valid {
		if _, err := normalizeDependencyRuntime(item); err != nil {
			t.Fatalf("runtime=%q 应该合法, err=%v", item, err)
		}
	}

	if _, err := normalizeDependencyRuntime("go"); err == nil {
		t.Fatalf("runtime=go 应该非法")
	}
}

func TestNormalizeDependencyAction(t *testing.T) {
	valid := []string{"install", "remove", " INSTALL "}
	for _, item := range valid {
		if _, err := normalizeDependencyAction(item); err != nil {
			t.Fatalf("action=%q 应该合法, err=%v", item, err)
		}
	}

	if _, err := normalizeDependencyAction("update"); err == nil {
		t.Fatalf("action=update 应该非法")
	}
}

func TestNormalizeDependencyPackage(t *testing.T) {
	if _, err := normalizeDependencyPackage("requests==2.31.0"); err != nil {
		t.Fatalf("package 应该合法, err=%v", err)
	}
	if _, err := normalizeDependencyPackage("@types/node@22"); err != nil {
		t.Fatalf("scope package 应该合法, err=%v", err)
	}
	if _, err := normalizeDependencyPackage(""); err == nil {
		t.Fatalf("空 package 应该非法")
	}
	if _, err := normalizeDependencyPackage("-rf"); err == nil {
		t.Fatalf("以 - 开头的 package 应该非法")
	}
	if _, err := normalizeDependencyPackage("left pad"); err == nil {
		t.Fatalf("包含空白字符的 package 应该非法")
	}
}

func TestParseNodeDependencies(t *testing.T) {
	raw := []byte(`[
  {
    "name": "callit-runtime-deps",
    "dependencies": {
      "zod": {"version": "3.0.0"},
      "axios": {"version": "1.7.0"}
    }
  }
]`)

	list, err := parseNodeDependencies(raw)
	if err != nil {
		t.Fatalf("parseNodeDependencies 失败: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("依赖数量不正确，期望 2，实际 %d", len(list))
	}
	if list[0].Name != "axios" || list[1].Name != "zod" {
		t.Fatalf("排序结果不正确: %#v", list)
	}
}

func TestParsePythonDependencies(t *testing.T) {
	raw := []byte(`[
  {"name":"pip","version":"25.3"},
  {"name":"setuptools","version":"70.0.0"},
  {"name":"requests","version":"2.32.0"}
]`)
	list, err := parsePythonDependencies(raw)
	if err != nil {
		t.Fatalf("parsePythonDependencies 失败: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("应过滤默认包，仅保留用户依赖，实际 %#v", list)
	}
	if list[0].Name != "requests" || list[0].Version != "2.32.0" {
		t.Fatalf("解析结果不正确: %#v", list[0])
	}
}

func TestDependencyTaskAcquireRelease(t *testing.T) {
	s := &Server{}
	if !s.tryAcquireDependencyTask() {
		t.Fatalf("第一次获取任务锁应成功")
	}
	if s.tryAcquireDependencyTask() {
		t.Fatalf("任务锁占用中时应失败")
	}
	s.releaseDependencyTask()
	if !s.tryAcquireDependencyTask() {
		t.Fatalf("释放后应可再次获取任务锁")
	}
}

func TestPythonDependencyVersionDir(t *testing.T) {
	s := &Server{dataDir: "/app/data"}

	got, err := s.pythonDependencyVersionDir()
	if err != nil {
		t.Fatalf("pythonDependencyVersionDir 失败: %v", err)
	}

	want := filepath.Join("/app/data", ".lib", "python", "venv")
	if got != want {
		t.Fatalf("Python 依赖目录不正确，got=%q want=%q", got, want)
	}
}

func TestResolvePythonVenvPaths(t *testing.T) {
	pythonDir := filepath.Join("/data", ".lib", "python", "venv")

	pythonPath, requirementsPath := resolvePythonVenvPaths(pythonDir)
	if pythonPath != filepath.Join(pythonDir, "bin", "python") {
		t.Fatalf("python 可执行文件路径不正确，got=%q", pythonPath)
	}
	if requirementsPath != filepath.Join("/data", ".lib", "python", "requirements.txt") {
		t.Fatalf("requirements.txt 路径不正确，got=%q", requirementsPath)
	}
}

func TestBuildPythonPipCommandUsesPythonModuleMode(t *testing.T) {
	pythonPath := filepath.Join("/data", ".lib", "python", "venv", "bin", "python")

	args := buildPythonPipArgs("install", "requests")
	got := append([]string{pythonPath}, args...)
	want := []string{pythonPath, "-m", "pip", "install", "requests"}
	if len(got) != len(want) {
		t.Fatalf("命令参数长度不正确，got=%v want=%v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("命令参数不正确，got=%v want=%v", got, want)
		}
	}
}

func TestEnsurePythonDependencyEnvReturnsBrokenMessage(t *testing.T) {
	s := &Server{dataDir: t.TempDir()}

	_, _, err := s.ensurePythonDependencyEnv(context.Background())
	if err == nil {
		t.Fatalf("缺少 venv 时应返回错误")
	}
	if !strings.Contains(err.Error(), "Python 运行环境已损坏，请进行环境重建") {
		t.Fatalf("错误信息不正确，got=%q", err.Error())
	}
}

func TestRebuildDependenciesIgnoresNodeRuntime(t *testing.T) {
	cfg := config.Config{
		AdminPrefix: "/admin",
		AdminToken:  "test-token",
		DataDir:     t.TempDir(),
	}
	engine := NewEngine(openAdminTestStore(t), router.New(), nil, &cfg)

	resp := doAdminJSONRequest(t, engine, http.MethodPost, "/admin/api/dependencies/rebuild?runtime=node", nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("node 重建接口应直接成功，code=%d body=%s", resp.Code, resp.Body.String())
	}

	body := resp.Body.String()
	if !strings.Contains(body, "event: done") || !strings.Contains(body, "\"ok\":true") {
		t.Fatalf("node 重建接口返回不正确: %s", body)
	}
}
