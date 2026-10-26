package maintenance

import (
	"context"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"callit/internal/db"
)

const (
	workerLogCleanupBatchSize  = 1000
	workerLogVacuumBatchPages  = 1000
	workerLogCleanupInterval   = 24 * time.Hour
	workerLogCleanupRetryDelay = 10 * time.Minute
)

type WorkerLogCleaner struct {
	store         *db.Store
	retentionDays atomic.Int64
	triggerCh     chan struct{}
	now           func() time.Time
}

type workerLogCleanupResult struct {
	deletedRows    int64
	reclaimedPages int64
}

func NewWorkerLogCleaner(store *db.Store, retentionDays int) *WorkerLogCleaner {
	cleaner := &WorkerLogCleaner{
		store:     store,
		triggerCh: make(chan struct{}, 1),
		now: func() time.Time {
			return time.Now().UTC()
		},
	}
	cleaner.retentionDays.Store(int64(retentionDays))
	return cleaner
}

// Prepare 在服务监听端口前完成现有数据库的一次性 auto_vacuum 模式转换。
func (cleaner *WorkerLogCleaner) Prepare(ctx context.Context) error {
	mode, err := cleaner.store.WorkerLog.AutoVacuumMode(ctx)
	if err != nil {
		return fmt.Errorf("读取 auto_vacuum 模式失败: %w", err)
	}
	hasCreatedAtIndex, err := cleaner.store.WorkerLog.HasCreatedAtIndex(ctx)
	if err != nil {
		return fmt.Errorf("检查 Worker 日志时间索引失败: %w", err)
	}
	if mode == 2 && hasCreatedAtIndex {
		return nil
	}

	startedAt := time.Now()
	var deletedRows int64
	if hasCreatedAtIndex {
		deletedRows, err = cleaner.deleteExpired(ctx)
	} else {
		cutoff := cleaner.now().UTC().AddDate(0, 0, -int(cleaner.retentionDays.Load()))
		deletedRows, err = cleaner.store.WorkerLog.DeleteBeforeByIDBatches(ctx, cutoff, workerLogCleanupBatchSize)
	}
	if err != nil {
		return fmt.Errorf("首次清理过期日志失败: %w", err)
	}
	if !hasCreatedAtIndex {
		if err := cleaner.store.WorkerLog.CreateCreatedAtIndex(ctx); err != nil {
			return fmt.Errorf("创建 Worker 日志时间索引失败: %w", err)
		}
	}
	if mode != 2 {
		if err := cleaner.store.WorkerLog.EnableIncrementalAutoVacuum(ctx); err != nil {
			return fmt.Errorf("启用增量回收失败: %w", err)
		}
	}
	slog.Info("完成日志数据库首次维护", "deleted_rows", deletedRows, "duration_ms", time.Since(startedAt).Milliseconds())
	return nil
}

func (cleaner *WorkerLogCleaner) Start(ctx context.Context) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		cleaner.loop(ctx)
	}()
	return done
}

func (cleaner *WorkerLogCleaner) UpdateRetentionDays(retentionDays int) {
	cleaner.retentionDays.Store(int64(retentionDays))
	select {
	case cleaner.triggerCh <- struct{}{}:
	default:
	}
}

func (cleaner *WorkerLogCleaner) loop(ctx context.Context) {
	timer := time.NewTimer(0)
	defer timer.Stop()

	retryUsed := false
	for {
		select {
		case <-ctx.Done():
			return
		case <-cleaner.triggerCh:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			retryUsed = false
		case <-timer.C:
		}

		startedAt := time.Now()
		result, err := cleaner.cleanup(ctx)
		if err != nil {
			slog.Error("清理过期 Worker 日志失败", "err", err, "duration_ms", time.Since(startedAt).Milliseconds())
			if !retryUsed {
				retryUsed = true
				timer.Reset(workerLogCleanupRetryDelay)
				continue
			}
		} else {
			slog.Info("完成过期 Worker 日志清理",
				"retention_days", cleaner.retentionDays.Load(),
				"deleted_rows", result.deletedRows,
				"reclaimed_pages", result.reclaimedPages,
				"duration_ms", time.Since(startedAt).Milliseconds(),
			)
		}

		retryUsed = false
		timer.Reset(workerLogCleanupInterval)
	}
}

func (cleaner *WorkerLogCleaner) cleanup(ctx context.Context) (workerLogCleanupResult, error) {
	deletedRows, err := cleaner.deleteExpired(ctx)
	if err != nil {
		return workerLogCleanupResult{}, err
	}

	mode, err := cleaner.store.WorkerLog.AutoVacuumMode(ctx)
	if err != nil {
		return workerLogCleanupResult{deletedRows: deletedRows}, err
	}
	if mode != 2 {
		slog.Warn("日志数据库尚未启用增量回收，将在下次启动时重试", "auto_vacuum_mode", mode)
		return workerLogCleanupResult{deletedRows: deletedRows}, nil
	}

	reclaimedPages, err := cleaner.store.WorkerLog.IncrementalVacuum(ctx, workerLogVacuumBatchPages)
	if err != nil {
		return workerLogCleanupResult{deletedRows: deletedRows}, err
	}
	return workerLogCleanupResult{deletedRows: deletedRows, reclaimedPages: reclaimedPages}, nil
}

func (cleaner *WorkerLogCleaner) deleteExpired(ctx context.Context) (int64, error) {
	retentionDays := int(cleaner.retentionDays.Load())
	cutoff := cleaner.now().UTC().AddDate(0, 0, -retentionDays)

	var deletedRows int64
	for {
		batchRows, err := cleaner.store.WorkerLog.DeleteBeforeBatch(ctx, cutoff, workerLogCleanupBatchSize)
		if err != nil {
			return deletedRows, err
		}
		deletedRows += batchRows
		if batchRows < workerLogCleanupBatchSize {
			return deletedRows, nil
		}
		if err := ctx.Err(); err != nil {
			return deletedRows, err
		}
	}
}
