package executor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSyncWorkerSDKFilesReplacesOldSDKFiles(t *testing.T) {
	runtimeLibDir := t.TempDir()

	oldPythonDir := filepath.Join(runtimeLibDir, "python", "callit")
	if err := os.MkdirAll(oldPythonDir, 0o755); err != nil {
		t.Fatalf("创建旧 Python SDK 目录失败: %v", err)
	}
	if err := os.WriteFile(filepath.Join(oldPythonDir, "callit.py"), []byte("legacy"), 0o644); err != nil {
		t.Fatalf("写入旧 Python SDK 文件失败: %v", err)
	}
	if err := os.WriteFile(filepath.Join(oldPythonDir, "__init__.py"), []byte("from .main import kv\n"), 0o644); err != nil {
		t.Fatalf("写入旧 Python __init__.py 失败: %v", err)
	}

	oldNodeDir := filepath.Join(runtimeLibDir, "node", "node_modules", "callit")
	if err := os.MkdirAll(oldNodeDir, 0o755); err != nil {
		t.Fatalf("创建旧 Node SDK 目录失败: %v", err)
	}
	if err := os.WriteFile(filepath.Join(oldNodeDir, "index.js"), []byte("module.exports = { kv };"), 0o644); err != nil {
		t.Fatalf("写入旧 Node SDK 文件失败: %v", err)
	}

	if err := SyncWorkerSDKFiles(runtimeLibDir); err != nil {
		t.Fatalf("同步 Worker SDK 失败: %v", err)
	}

	if _, err := os.Stat(filepath.Join(oldPythonDir, "callit.py")); !os.IsNotExist(err) {
		t.Fatalf("旧 Python SDK 文件应被清理，err=%v", err)
	}

	pythonInitContent, err := os.ReadFile(filepath.Join(oldPythonDir, "__init__.py"))
	if err != nil {
		t.Fatalf("读取 Python __init__.py 失败: %v", err)
	}
	if !strings.Contains(string(pythonInitContent), "db") {
		t.Fatalf("Python SDK 应包含 db 导出，content=%q", string(pythonInitContent))
	}

	nodeSDKContent, err := os.ReadFile(filepath.Join(oldNodeDir, "index.js"))
	if err != nil {
		t.Fatalf("读取 Node SDK 文件失败: %v", err)
	}
	if !strings.Contains(string(nodeSDKContent), "const { db } = require(\"./db\");") {
		t.Fatalf("Node SDK 应包含 db 导出，content=%q", string(nodeSDKContent))
	}
}
