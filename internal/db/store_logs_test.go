package db

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"callit/internal/model"
)

func TestListWorkerLogsPaged(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatalf("打开数据库失败: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	ctx := context.Background()
	workerID := "worker-1"
	for i := 1; i <= 5; i++ {
		if err := store.InsertWorkerLog(ctx, model.WorkerLog{
			ID:         fmt.Sprintf("log-%03d", i),
			WorkerID:   workerID,
			RequestID:  fmt.Sprintf("req-%03d", i),
			Status:     200 + i,
			Stdout:     "stdout",
			Stderr:     "",
			Error:      "",
			DurationMS: int64(i),
		}); err != nil {
			t.Fatalf("插入日志失败: %v", err)
		}
	}
	if err := store.InsertWorkerLog(ctx, model.WorkerLog{
		ID:         "log-other",
		WorkerID:   "worker-2",
		RequestID:  "req-other",
		Status:     200,
		Stdout:     "other",
		DurationMS: 1,
	}); err != nil {
		t.Fatalf("插入其他 worker 日志失败: %v", err)
	}

	page1, total, err := store.ListWorkerLogsPaged(ctx, workerID, 1, 2)
	if err != nil {
		t.Fatalf("查询第一页失败: %v", err)
	}
	if total != 5 {
		t.Fatalf("total 不正确，期望 5，实际 %d", total)
	}
	if len(page1) != 2 {
		t.Fatalf("第一页条数不正确，期望 2，实际 %d", len(page1))
	}
	if page1[0].ID != "log-005" || page1[1].ID != "log-004" {
		t.Fatalf("第一页排序不正确: %#v", page1)
	}

	page3, total, err := store.ListWorkerLogsPaged(ctx, workerID, 3, 2)
	if err != nil {
		t.Fatalf("查询第三页失败: %v", err)
	}
	if total != 5 {
		t.Fatalf("total 不正确，期望 5，实际 %d", total)
	}
	if len(page3) != 1 || page3[0].ID != "log-001" {
		t.Fatalf("第三页数据不正确: %#v", page3)
	}
}
