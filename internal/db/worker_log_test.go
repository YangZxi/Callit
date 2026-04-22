package db

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"callit/internal/model"
)

func openWorkerLogTestStore(t *testing.T) *Store {
	t.Helper()

	store, err := Open(filepath.Join(t.TempDir(), "app.db"))
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

func TestLatestPerWorkerUsesLatestInsertedLog(t *testing.T) {
	ctx := context.Background()
	store := openWorkerLogTestStore(t)

	if _, err := store.Worker.Create(ctx, model.Worker{
		ID:        "worker-latest",
		Name:      "最新日志测试",
		Runtime:   "python",
		Route:     "/worker-latest",
		TimeoutMS: 3000,
		Enabled:   true,
	}); err != nil {
		t.Fatalf("创建 Worker 失败: %v", err)
	}

	baseTime := time.Date(2026, 4, 22, 10, 0, 0, 0, time.UTC)
	if err := store.WorkerLog.Insert(ctx, model.WorkerLog{
		WorkerID:   "worker-latest",
		RequestID:  "failed-earlier-insert",
		Status:     500,
		Error:      "脚本失败",
		DurationMS: 100,
		CreatedAt:  baseTime.Add(time.Hour),
	}); err != nil {
		t.Fatalf("插入失败日志失败: %v", err)
	}
	if err := store.WorkerLog.Insert(ctx, model.WorkerLog{
		WorkerID:   "worker-latest",
		RequestID:  "success-later-insert",
		Status:     200,
		DurationMS: 120,
		CreatedAt:  baseTime,
	}); err != nil {
		t.Fatalf("插入成功日志失败: %v", err)
	}

	logs, err := store.WorkerLog.LatestPerWorker(ctx)
	if err != nil {
		t.Fatalf("查询每个 Worker 最新日志失败: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("最新日志数量不正确，got=%d want=1 logs=%#v", len(logs), logs)
	}
	if logs[0].RequestID != "success-later-insert" {
		t.Fatalf("应按插入顺序选择最后一条日志，got=%q", logs[0].RequestID)
	}
	if !logs[0].IsSuccess() {
		t.Fatalf("最后一条日志应为成功日志: %#v", logs[0])
	}
}
