package migrate

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"callit/internal/config"
	"callit/internal/db"
	"callit/internal/model"
)

func openMigrateTestStore(t *testing.T) *db.Store {
	t.Helper()
	store, err := db.Open(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatalf("打开测试数据库失败: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("关闭测试数据库失败: %v", err)
		}
	})
	return store
}

func createMigrateTestWorker(t *testing.T, store *db.Store, worker model.Worker) {
	t.Helper()
	if _, err := store.Worker.Create(context.Background(), worker); err != nil {
		t.Fatalf("创建测试 Worker 失败: %v", err)
	}
}

func TestServiceRebuildWorkerDirStructureMigratesLegacyWorkerLayout(t *testing.T) {
	baseDir := t.TempDir()
	cfg := config.Config{
		DataDir:              baseDir,
		WorkersDir:           filepath.Join(baseDir, "workers"),
		WorkerRunningTempDir: filepath.Join(baseDir, "tmp"),
		RuntimeLibDir:        filepath.Join(baseDir, ".lib"),
	}
	if err := os.MkdirAll(cfg.WorkersDir, 0o755); err != nil {
		t.Fatalf("创建 workers 目录失败: %v", err)
	}
	legacyDir := filepath.Join(cfg.WorkersDir, "worker-1")
	if err := os.MkdirAll(legacyDir, 0o755); err != nil {
		t.Fatalf("创建旧版 worker 目录失败: %v", err)
	}
	if err := os.WriteFile(filepath.Join(legacyDir, "main.py"), []byte("print('ok')"), 0o644); err != nil {
		t.Fatalf("写入旧版 main.py 失败: %v", err)
	}
	if err := os.WriteFile(filepath.Join(legacyDir, "index.html"), []byte("<h1>ok</h1>"), 0o644); err != nil {
		t.Fatalf("写入旧版 index.html 失败: %v", err)
	}

	store := openMigrateTestStore(t)
	createMigrateTestWorker(t, store, model.Worker{
		ID:        "worker-1",
		Name:      "迁移 Worker",
		Runtime:   "python",
		Route:     "/worker-1",
		TimeoutMS: 3000,
		Enabled:   true,
	})

	service := NewService(store, cfg)
	if err := service.RebuildWorkerDirStructure(context.Background()); err != nil {
		t.Fatalf("执行迁移失败: %v", err)
	}

	for _, path := range []string{
		filepath.Join(legacyDir, "code", "main.py"),
		filepath.Join(legacyDir, "code", "index.html"),
		filepath.Join(legacyDir, "data"),
		filepath.Join(legacyDir, "metadata.json"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("迁移后缺少路径 %s: %v", path, err)
		}
	}
	if _, err := os.Stat(filepath.Join(legacyDir, "main.py")); !os.IsNotExist(err) {
		t.Fatalf("旧版 main.py 应已移入 code 目录，err=%v", err)
	}

	backupEntries, err := filepath.Glob(filepath.Join(cfg.WorkerRunningTempDir, "workers-migration-*", "workers", "worker-1", "main.py"))
	if err != nil {
		t.Fatalf("查找备份目录失败: %v", err)
	}
	if len(backupEntries) != 1 {
		t.Fatalf("应生成一次整目录备份，got=%v", backupEntries)
	}

	raw, err := os.ReadFile(filepath.Join(legacyDir, "metadata.json"))
	if err != nil {
		t.Fatalf("读取 metadata 失败: %v", err)
	}
	var metadata model.Worker
	if err := json.Unmarshal(raw, &metadata); err != nil {
		t.Fatalf("解析 metadata 失败: %v", err)
	}
	if metadata.ID != "worker-1" || metadata.Name != "迁移 Worker" {
		t.Fatalf("metadata 不正确: %#v", metadata)
	}
}

func TestServiceRebuildWorkerDirStructureIsIdempotentForNewLayout(t *testing.T) {
	baseDir := t.TempDir()
	cfg := config.Config{
		DataDir:              baseDir,
		WorkersDir:           filepath.Join(baseDir, "workers"),
		WorkerRunningTempDir: filepath.Join(baseDir, "tmp"),
		RuntimeLibDir:        filepath.Join(baseDir, ".lib"),
	}
	store := openMigrateTestStore(t)
	createMigrateTestWorker(t, store, model.Worker{
		ID:        "worker-2",
		Name:      "新版 Worker",
		Runtime:   "node",
		Route:     "/worker-2",
		TimeoutMS: 3000,
		Enabled:   false,
	})

	workerDir := filepath.Join(cfg.WorkersDir, "worker-2")
	if err := os.MkdirAll(filepath.Join(workerDir, "code"), 0o755); err != nil {
		t.Fatalf("创建 code 目录失败: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workerDir, "code", "main.js"), []byte("module.exports={}"), 0o644); err != nil {
		t.Fatalf("写入 main.js 失败: %v", err)
	}

	service := NewService(store, cfg)
	if err := service.RebuildWorkerDirStructure(context.Background()); err != nil {
		t.Fatalf("首次执行迁移失败: %v", err)
	}
	if err := service.RebuildWorkerDirStructure(context.Background()); err != nil {
		t.Fatalf("二次执行迁移失败: %v", err)
	}

	if _, err := os.Stat(filepath.Join(workerDir, "data")); err != nil {
		t.Fatalf("应补齐 data 目录: %v", err)
	}
	if _, err := os.Stat(filepath.Join(workerDir, "metadata.json")); err != nil {
		t.Fatalf("应补齐 metadata.json: %v", err)
	}
	backupEntries, err := filepath.Glob(filepath.Join(cfg.WorkerRunningTempDir, "workers-migration-*"))
	if err != nil {
		t.Fatalf("查找备份目录失败: %v", err)
	}
	if len(backupEntries) != 0 {
		t.Fatalf("新版目录不应触发备份: %v", backupEntries)
	}
}
