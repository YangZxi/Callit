package executor

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAppendOrMergeEnvPath(t *testing.T) {
	env := []string{"A=1"}
	updated := appendOrMergeEnvPath(env, "NODE_PATH", "/tmp/a")
	if len(updated) != 2 {
		t.Fatalf("环境变量数量不正确: %d", len(updated))
	}

	updated = appendOrMergeEnvPath(updated, "NODE_PATH", "/tmp/a")
	if len(updated) != 2 {
		t.Fatalf("重复路径不应追加: %#v", updated)
	}

	updated = appendOrMergeEnvPath(updated, "NODE_PATH", "/tmp/b")
	found := false
	for _, item := range updated {
		if item == "NODE_PATH=/tmp/a"+string(os.PathListSeparator)+"/tmp/b" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("路径合并失败: %#v", updated)
	}
}

func TestDetectPythonSitePackages(t *testing.T) {
	dir := t.TempDir()
	site := filepath.Join(dir, "lib", "python3.12", "site-packages")
	if err := os.MkdirAll(site, 0o755); err != nil {
		t.Fatalf("创建目录失败: %v", err)
	}

	got := detectPythonSitePackages(dir)
	if got != site {
		t.Fatalf("detectPythonSitePackages 结果错误: got=%s want=%s", got, site)
	}
}
