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

func TestWorkerLogDashboardAggregates(t *testing.T) {
	ctx := context.Background()
	store := openWorkerLogTestStore(t)
	if _, err := store.Worker.Create(ctx, model.Worker{
		ID:        "worker-aggregate",
		Name:      "聚合测试",
		Runtime:   "python",
		Route:     "/worker-aggregate",
		TimeoutMS: 3000,
		Enabled:   true,
	}); err != nil {
		t.Fatalf("创建 Worker 失败: %v", err)
	}

	start := time.Date(2026, 10, 26, 10, 0, 0, 0, time.UTC)
	logs := []model.WorkerLog{
		{WorkerID: "worker-aggregate", RequestID: "success-200", Status: 200, DurationMS: 100, CreatedAt: start.Add(10 * time.Minute)},
		{WorkerID: "worker-aggregate", RequestID: "success-399", Status: 399, DurationMS: 300, CreatedAt: start.Add(20 * time.Minute)},
		{WorkerID: "worker-aggregate", RequestID: "failed-500", Status: 500, DurationMS: 500, CreatedAt: start.Add(70 * time.Minute)},
		{WorkerID: "worker-aggregate", RequestID: "failed-error", Status: 200, Error: "执行失败", DurationMS: 700, CreatedAt: start.Add(80 * time.Minute)},
		{WorkerID: "worker-aggregate", RequestID: "excluded-boundary", Status: 200, DurationMS: 900, CreatedAt: start.Add(2 * time.Hour)},
	}
	for _, logEntry := range logs {
		if err := store.WorkerLog.Insert(ctx, logEntry); err != nil {
			t.Fatalf("插入聚合测试日志失败: %v", err)
		}
	}

	finish := start.Add(2 * time.Hour)
	summary, err := store.WorkerLog.SummaryBetween(ctx, start, finish)
	if err != nil {
		t.Fatalf("查询日志摘要失败: %v", err)
	}
	if summary.Total != 4 || summary.Success != 2 || summary.Failed != 2 || summary.DurationTotal != 1600 {
		t.Fatalf("日志摘要错误: %#v", summary)
	}

	aggregates, err := store.WorkerLog.AggregateByWorkerBetween(ctx, start, finish)
	if err != nil {
		t.Fatalf("查询 Worker 聚合失败: %v", err)
	}
	if len(aggregates) != 1 || aggregates[0].Total != 4 || aggregates[0].DurationTotal != 1600 {
		t.Fatalf("Worker 聚合错误: %#v", aggregates)
	}

	trend, err := store.WorkerLog.TrendBetween(ctx, "worker-aggregate", start, finish)
	if err != nil {
		t.Fatalf("查询日志趋势失败: %v", err)
	}
	if len(trend) != 2 {
		t.Fatalf("日志趋势桶数量错误: %#v", trend)
	}
	if trend[0].HourUnix != start.Unix()/3600 || trend[0].Total != 2 || trend[0].Success != 2 {
		t.Fatalf("第一个趋势桶错误: %#v", trend[0])
	}
	if trend[1].HourUnix != start.Add(time.Hour).Unix()/3600 || trend[1].Total != 2 || trend[1].Failed != 2 {
		t.Fatalf("第二个趋势桶错误: %#v", trend[1])
	}
}

func TestDeleteBeforeByIDBatchesHandlesOutOfOrderTimestamps(t *testing.T) {
	ctx := context.Background()
	store := openWorkerLogTestStore(t)
	if _, err := store.Worker.Create(ctx, model.Worker{
		ID:        "worker-delete-by-id",
		Name:      "分批删除测试",
		Runtime:   "python",
		Route:     "/worker-delete-by-id",
		TimeoutMS: 3000,
		Enabled:   true,
	}); err != nil {
		t.Fatalf("创建 Worker 失败: %v", err)
	}

	cutoff := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	logs := []model.WorkerLog{
		{WorkerID: "worker-delete-by-id", RequestID: "old-first", Status: 200, CreatedAt: cutoff.Add(-time.Hour)},
		{WorkerID: "worker-delete-by-id", RequestID: "new-middle", Status: 200, CreatedAt: cutoff.Add(time.Hour)},
		{WorkerID: "worker-delete-by-id", RequestID: "old-last", Status: 200, CreatedAt: cutoff.Add(-2 * time.Hour)},
	}
	for _, logEntry := range logs {
		if err := store.WorkerLog.Insert(ctx, logEntry); err != nil {
			t.Fatalf("插入日志失败: %v", err)
		}
	}

	deletedRows, err := store.WorkerLog.DeleteBeforeByIDBatches(ctx, cutoff, 2)
	if err != nil {
		t.Fatalf("按 ID 分批删除失败: %v", err)
	}
	if deletedRows != 2 {
		t.Fatalf("删除数量错误: got=%d want=2", deletedRows)
	}
	remaining, total, err := store.WorkerLog.ListPaged(ctx, "worker-delete-by-id", 1, 10)
	if err != nil {
		t.Fatalf("读取剩余日志失败: %v", err)
	}
	if total != 1 || remaining[0].RequestID != "new-middle" {
		t.Fatalf("分批删除保留结果错误: %#v", remaining)
	}
}
