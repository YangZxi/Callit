package worker

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"callit/internal/model"
)

func TestWorkerSpecEnsureLayoutAndMetadata(t *testing.T) {
	baseDir := t.TempDir()
	spec, err := NewRuntimeWorkerSpec(
		filepath.Join(baseDir, "workers"),
		filepath.Join(baseDir, "tmp"),
		filepath.Join(baseDir, ".lib"),
		model.Worker{
			ID:        "worker-1",
			Name:      "测试 Worker",
			Runtime:   "python",
			Route:     "/worker-1",
			TimeoutMS: 3000,
			Enabled:   true,
		},
		"req-1",
	)
	if err != nil {
		t.Fatalf("NewRuntimeWorkerSpec 失败: %v", err)
	}

	if err := spec.EnsureLayout(); err != nil {
		t.Fatalf("EnsureLayout 失败: %v", err)
	}
	if err := spec.WriteMetadata(); err != nil {
		t.Fatalf("WriteMetadata 失败: %v", err)
	}

	for _, path := range []string{spec.WorkerRootDir, spec.WorkerCodeDir, spec.WorkerDataDir} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("目录不存在 %s: %v", path, err)
		}
		if !info.IsDir() {
			t.Fatalf("路径不是目录: %s", path)
		}
	}
	if spec.WorkerTmpDir != filepath.Join(baseDir, "tmp", "req-1") {
		t.Fatalf("WorkerTmpDir 不正确: %s", spec.WorkerTmpDir)
	}

	raw, err := os.ReadFile(spec.MetadataPath)
	if err != nil {
		t.Fatalf("读取 metadata 失败: %v", err)
	}
	var got model.Worker
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("解析 metadata 失败: %v", err)
	}
	if got.ID != spec.Worker.ID || got.Name != spec.Worker.Name || got.Runtime != spec.Worker.Runtime {
		t.Fatalf("metadata 内容不正确: %#v", got)
	}
}

func TestWorkerSpecListCodeFilesOnlyReadsCodeDir(t *testing.T) {
	baseDir := t.TempDir()
	spec := NewWorkerSpec(filepath.Join(baseDir, "workers"), filepath.Join(baseDir, ".lib"), model.Worker{
		ID:        "worker-2",
		Name:      "测试 Worker",
		Runtime:   "node",
		Route:     "/worker-2",
		TimeoutMS: 3000,
		Enabled:   true,
	})
	if spec.WorkerTmpDir != "" {
		t.Fatalf("静态 WorkerSpec 不应包含 WorkerTmpDir: %q", spec.WorkerTmpDir)
	}
	if err := spec.EnsureLayout(); err != nil {
		t.Fatalf("EnsureLayout 失败: %v", err)
	}
	if err := os.WriteFile(filepath.Join(spec.WorkerCodeDir, "index.html"), []byte("ok"), 0o644); err != nil {
		t.Fatalf("写入 code 文件失败: %v", err)
	}
	if err := os.WriteFile(filepath.Join(spec.WorkerRootDir, "ignore.txt"), []byte("ignore"), 0o644); err != nil {
		t.Fatalf("写入 root 文件失败: %v", err)
	}

	files, err := spec.ListCodeFiles()
	if err != nil {
		t.Fatalf("ListCodeFiles 失败: %v", err)
	}
	if len(files) != 1 || files[0] != "index.html" {
		t.Fatalf("代码目录文件列表不正确: %#v", files)
	}
}

func TestRenameCodeFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "old.py"), []byte("1"), 0o644); err != nil {
		t.Fatalf("写入原文件失败: %v", err)
	}
	if err := RenameCodeFile(dir, "old.py", "new.py"); err != nil {
		t.Fatalf("RenameCodeFile 失败: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "new.py")); err != nil {
		t.Fatalf("重命名后文件不存在: %v", err)
	}
}

func TestRenameCodeFileReturnsSentinelErrors(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "old.py"), []byte("1"), 0o644); err != nil {
		t.Fatalf("写入原文件失败: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "new.py"), []byte("1"), 0o644); err != nil {
		t.Fatalf("写入目标文件失败: %v", err)
	}

	err := RenameCodeFile(dir, "old.py", "new.py")
	if !errors.Is(err, ErrTargetFileExists) {
		t.Fatalf("应返回 ErrTargetFileExists，got=%v", err)
	}
	err = RenameCodeFile(dir, "missing.py", "new.py")
	if !errors.Is(err, ErrSourceFileNotExist) {
		t.Fatalf("应返回 ErrSourceFileNotExist，got=%v", err)
	}
}

func TestNewRuntimeWorkerSpecRequiresRequestID(t *testing.T) {
	baseDir := t.TempDir()
	_, err := NewRuntimeWorkerSpec(
		filepath.Join(baseDir, "workers"),
		filepath.Join(baseDir, "tmp"),
		filepath.Join(baseDir, ".lib"),
		model.Worker{
			ID:        "worker-3",
			Name:      "测试 Worker",
			Runtime:   "python",
			Route:     "/worker-3",
			TimeoutMS: 3000,
			Enabled:   true,
		},
		"",
	)
	if err == nil {
		t.Fatalf("空 requestID 应返回错误")
	}
}
