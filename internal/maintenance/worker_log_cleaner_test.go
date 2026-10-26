package maintenance

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"callit/internal/db"
	"callit/internal/model"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func openMaintenanceTestStore(t *testing.T) (*db.Store, string) {
	t.Helper()
	databasePath := filepath.Join(t.TempDir(), "app.db")
	store, err := db.Open(databasePath)
	if err != nil {
		t.Fatalf("打开测试数据库失败: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("关闭测试数据库失败: %v", err)
		}
	})
	return store, databasePath
}

func createMaintenanceTestWorker(t *testing.T, store *db.Store, workerID string) {
	t.Helper()
	if _, err := store.Worker.Create(context.Background(), model.Worker{
		ID:        workerID,
		Name:      workerID,
		Runtime:   "python",
		Route:     "/" + workerID,
		TimeoutMS: 3000,
		Enabled:   true,
	}); err != nil {
		t.Fatalf("创建测试 Worker 失败: %v", err)
	}
}

func TestWorkerLogCleanerDeletesOnlyExpiredLogs(t *testing.T) {
	store, _ := openMaintenanceTestStore(t)
	createMaintenanceTestWorker(t, store, "worker-cleanup")
	mode, err := store.WorkerLog.AutoVacuumMode(context.Background())
	if err != nil {
		t.Fatalf("读取新数据库 auto_vacuum 模式失败: %v", err)
	}
	if mode != 2 {
		t.Fatalf("新数据库应启用增量回收: got=%d want=2", mode)
	}

	now := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	cutoff := now.AddDate(0, 0, -30)
	logs := []model.WorkerLog{
		{WorkerID: "worker-cleanup", RequestID: "expired", Status: 200, CreatedAt: cutoff.Add(-time.Second)},
		{WorkerID: "worker-cleanup", RequestID: "boundary", Status: 200, CreatedAt: cutoff},
		{WorkerID: "worker-cleanup", RequestID: "recent", Status: 200, CreatedAt: now.Add(-time.Hour)},
	}
	for _, logEntry := range logs {
		if err := store.WorkerLog.Insert(context.Background(), logEntry); err != nil {
			t.Fatalf("写入测试日志失败: %v", err)
		}
	}

	cleaner := NewWorkerLogCleaner(store, 30)
	cleaner.now = func() time.Time { return now }
	result, err := cleaner.cleanup(context.Background())
	if err != nil {
		t.Fatalf("执行日志清理失败: %v", err)
	}
	if result.deletedRows != 1 {
		t.Fatalf("删除日志数量错误: got=%d want=1", result.deletedRows)
	}

	remaining, total, err := store.WorkerLog.ListPaged(context.Background(), "worker-cleanup", 1, 10)
	if err != nil {
		t.Fatalf("读取剩余日志失败: %v", err)
	}
	if total != 2 {
		t.Fatalf("剩余日志数量错误: got=%d want=2", total)
	}
	for _, logEntry := range remaining {
		if logEntry.RequestID == "expired" {
			t.Fatalf("过期日志仍然存在: %#v", logEntry)
		}
	}
}

func TestWorkerLogCleanerStopsWithContext(t *testing.T) {
	store, _ := openMaintenanceTestStore(t)
	cleaner := NewWorkerLogCleaner(store, 30)
	if err := cleaner.Prepare(context.Background()); err != nil {
		t.Fatalf("准备测试数据库失败: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := cleaner.Start(ctx)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("取消上下文后日志清理协程未退出")
	}
}

func TestWorkerLogCleanerPrepareConvertsAndShrinksLegacyDatabase(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "legacy.db")
	legacyDB, err := gorm.Open(sqlite.Open(databasePath), &gorm.Config{})
	if err != nil {
		t.Fatalf("创建旧版数据库失败: %v", err)
	}
	if err := legacyDB.Exec("PRAGMA auto_vacuum = NONE;").Error; err != nil {
		t.Fatalf("设置旧版 auto_vacuum 模式失败: %v", err)
	}
	if err := legacyDB.Exec("CREATE TABLE legacy_marker (id INTEGER PRIMARY KEY);").Error; err != nil {
		t.Fatalf("创建旧版数据库标记表失败: %v", err)
	}
	legacySQLDB, err := legacyDB.DB()
	if err != nil {
		t.Fatalf("获取旧版数据库连接失败: %v", err)
	}
	if err := legacySQLDB.Close(); err != nil {
		t.Fatalf("关闭旧版数据库失败: %v", err)
	}

	store, err := db.Open(databasePath)
	if err != nil {
		t.Fatalf("打开旧版数据库失败: %v", err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			t.Fatalf("关闭测试数据库失败: %v", err)
		}
	}()

	mode, err := store.WorkerLog.AutoVacuumMode(context.Background())
	if err != nil {
		t.Fatalf("读取转换前模式失败: %v", err)
	}
	if mode != 0 {
		t.Fatalf("旧版数据库转换前模式错误: got=%d want=0", mode)
	}
	hasIndex, err := store.WorkerLog.HasCreatedAtIndex(context.Background())
	if err != nil {
		t.Fatalf("检查转换前索引失败: %v", err)
	}
	if hasIndex {
		t.Fatal("旧版数据库不应在清理前提前创建时间索引")
	}

	createMaintenanceTestWorker(t, store, "worker-legacy-cleanup")
	largeOutput := strings.Repeat("x", 64*1024)
	for index := 0; index < 20; index++ {
		if err := store.WorkerLog.Insert(context.Background(), model.WorkerLog{
			WorkerID:  "worker-legacy-cleanup",
			RequestID: "legacy-expired-" + time.Unix(int64(index), 0).Format("150405"),
			Status:    200,
			ExecLog:   largeOutput,
			CreatedAt: time.Date(2026, 8, 1, 0, 0, index, 0, time.UTC),
		}); err != nil {
			t.Fatalf("写入旧日志失败: %v", err)
		}
	}
	before, err := os.Stat(databasePath)
	if err != nil {
		t.Fatalf("读取压缩前数据库大小失败: %v", err)
	}

	cleaner := NewWorkerLogCleaner(store, 30)
	cleaner.now = func() time.Time {
		return time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	}
	if err := cleaner.Prepare(context.Background()); err != nil {
		t.Fatalf("准备旧版数据库失败: %v", err)
	}

	mode, err = store.WorkerLog.AutoVacuumMode(context.Background())
	if err != nil {
		t.Fatalf("读取转换后模式失败: %v", err)
	}
	if mode != 2 {
		t.Fatalf("数据库应转换为增量回收模式: got=%d want=2", mode)
	}
	hasIndex, err = store.WorkerLog.HasCreatedAtIndex(context.Background())
	if err != nil {
		t.Fatalf("检查转换后索引失败: %v", err)
	}
	if !hasIndex {
		t.Fatal("旧版数据库转换后应存在时间索引")
	}
	after, err := os.Stat(databasePath)
	if err != nil {
		t.Fatalf("读取压缩后数据库大小失败: %v", err)
	}
	if after.Size() >= before.Size() {
		t.Fatalf("完整压缩后数据库文件应缩小: before=%d after=%d", before.Size(), after.Size())
	}
}
