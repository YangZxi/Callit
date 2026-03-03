package admin

import "testing"

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
