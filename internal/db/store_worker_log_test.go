package db

import (
	"context"
	"path/filepath"
	"testing"

	"callit/internal/model"
)

func TestWorkerLogResultPersisted(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "app.db")
	store, err := Open(dbPath)
	if err != nil {
		t.Fatalf("打开数据库失败: %v", err)
	}
	defer store.Close()

	entry := model.WorkerLog{
		ID:         "log-1",
		WorkerID:   "worker-1",
		RequestID:  "req-1",
		Status:     200,
		Stdout:     "stdout-log",
		Stderr:     "stderr-log",
		Result:     `{"ok":true}`,
		Error:      "",
		DurationMS: 12,
	}
	if err := store.InsertWorkerLog(context.Background(), entry); err != nil {
		t.Fatalf("写入 Worker 日志失败: %v", err)
	}

	logs, total, err := store.ListWorkerLogsPaged(context.Background(), "worker-1", 1, 10)
	if err != nil {
		t.Fatalf("查询 Worker 日志失败: %v", err)
	}
	if total != 1 {
		t.Fatalf("日志总数错误: got=%d want=1", total)
	}
	if len(logs) != 1 {
		t.Fatalf("日志条数错误: got=%d want=1", len(logs))
	}
	if logs[0].Result != entry.Result {
		t.Fatalf("result 未正确持久化: got=%q want=%q", logs[0].Result, entry.Result)
	}
}
