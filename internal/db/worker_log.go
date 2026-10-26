package db

import (
	"context"
	"fmt"
	"time"

	"callit/internal/model"

	"gorm.io/gorm"
)

type WorkerLogDAO struct {
	db *gorm.DB
}

const workerLogCreatedAtIndex = "idx_worker_run_log_created"

// WorkerLogAggregate 表示一个 Worker 在指定时间窗口内的运行聚合。
type WorkerLogAggregate struct {
	WorkerID      string
	Total         int
	Success       int
	Failed        int
	DurationTotal int64
}

type WorkerLogSummary struct {
	Total         int
	Success       int
	Failed        int
	DurationTotal int64
}

type WorkerLogTrendAggregate struct {
	HourUnix      int64
	Total         int
	Success       int
	Failed        int
	DurationTotal int64
}

func (dao *WorkerLogDAO) Insert(ctx context.Context, log model.WorkerLog) error {
	if log.CreatedAt.IsZero() {
		log.CreatedAt = time.Now().UTC()
	}
	if log.Trigger == "" {
		log.Trigger = model.WorkerTriggerHTTP
	}
	return dao.db.WithContext(ctx).Create(&log).Error
}

func (dao *WorkerLogDAO) DeleteBeforeBatch(ctx context.Context, cutoff time.Time, limit int) (int64, error) {
	result := dao.db.WithContext(ctx).Exec(`
DELETE FROM worker_run_log
WHERE id IN (
	SELECT id
	FROM worker_run_log
	WHERE created_at < ?
	ORDER BY created_at ASC, id ASC
	LIMIT ?
)`, cutoff.UTC(), limit)
	return result.RowsAffected, result.Error
}

func (dao *WorkerLogDAO) DeleteBeforeByIDBatches(ctx context.Context, cutoff time.Time, limit int) (int64, error) {
	type workerLogTimestamp struct {
		ID        int64
		CreatedAt time.Time
	}

	var deletedRows int64
	var lastID int64
	for {
		var logs []workerLogTimestamp
		if err := dao.db.WithContext(ctx).
			Table(model.WorkerLog{}.TableName()).
			Select("id", "created_at").
			Where("id > ?", lastID).
			Order("id ASC").
			Limit(limit).
			Scan(&logs).Error; err != nil {
			return deletedRows, err
		}
		if len(logs) == 0 {
			return deletedRows, nil
		}

		expiredIDs := make([]int64, 0, len(logs))
		for _, logEntry := range logs {
			lastID = logEntry.ID
			if logEntry.CreatedAt.UTC().Before(cutoff.UTC()) {
				expiredIDs = append(expiredIDs, logEntry.ID)
			}
		}
		if len(expiredIDs) > 0 {
			result := dao.db.WithContext(ctx).Where("id IN ?", expiredIDs).Delete(&model.WorkerLog{})
			if result.Error != nil {
				return deletedRows, result.Error
			}
			deletedRows += result.RowsAffected
		}
		if len(logs) < limit {
			return deletedRows, nil
		}
		if err := ctx.Err(); err != nil {
			return deletedRows, err
		}
	}
}

func (dao *WorkerLogDAO) HasCreatedAtIndex(ctx context.Context) (bool, error) {
	var count int64
	err := dao.db.WithContext(ctx).Raw(
		"SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = ?",
		workerLogCreatedAtIndex,
	).Scan(&count).Error
	return count > 0, err
}

func (dao *WorkerLogDAO) CreateCreatedAtIndex(ctx context.Context) error {
	return dao.db.WithContext(ctx).Exec(
		"CREATE INDEX IF NOT EXISTS " + workerLogCreatedAtIndex + " ON worker_run_log(created_at)",
	).Error
}

func (dao *WorkerLogDAO) AutoVacuumMode(ctx context.Context) (int, error) {
	var mode int
	if err := dao.db.WithContext(ctx).Raw("PRAGMA auto_vacuum;").Scan(&mode).Error; err != nil {
		return 0, err
	}
	return mode, nil
}

func (dao *WorkerLogDAO) EnableIncrementalAutoVacuum(ctx context.Context) error {
	sqlDB, err := dao.db.DB()
	if err != nil {
		return err
	}
	conn, err := sqlDB.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()

	if _, err := conn.ExecContext(ctx, "PRAGMA auto_vacuum = INCREMENTAL;"); err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, "VACUUM;"); err != nil {
		return err
	}
	var mode int
	if err := conn.QueryRowContext(ctx, "PRAGMA auto_vacuum;").Scan(&mode); err != nil {
		return err
	}
	if mode != 2 {
		return fmt.Errorf("auto_vacuum 模式未切换为 INCREMENTAL，当前值为 %d", mode)
	}
	return nil
}

func (dao *WorkerLogDAO) IncrementalVacuum(ctx context.Context, batchPages int) (int64, error) {
	var reclaimed int64
	for {
		var before int64
		if err := dao.db.WithContext(ctx).Raw("PRAGMA freelist_count;").Scan(&before).Error; err != nil {
			return reclaimed, err
		}
		if before == 0 {
			return reclaimed, nil
		}
		if err := dao.db.WithContext(ctx).Exec(fmt.Sprintf("PRAGMA incremental_vacuum(%d);", batchPages)).Error; err != nil {
			return reclaimed, err
		}
		var after int64
		if err := dao.db.WithContext(ctx).Raw("PRAGMA freelist_count;").Scan(&after).Error; err != nil {
			return reclaimed, err
		}
		if after >= before {
			return reclaimed, nil
		}
		reclaimed += before - after
	}
}

func (dao *WorkerLogDAO) ListPaged(ctx context.Context, workerID string, page int, pageSize int) ([]model.WorkerLog, int, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}

	query := dao.db.WithContext(ctx).Model(&model.WorkerLog{}).Where("worker_id = ?", workerID)

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var logs []model.WorkerLog
	offset := (page - 1) * pageSize
	if err := dao.db.WithContext(ctx).
		Where("worker_id = ?", workerID).
		Order("created_at DESC, id DESC").
		Limit(pageSize).
		Offset(offset).
		Find(&logs).Error; err != nil {
		return nil, 0, err
	}
	return logs, int(total), nil
}

func (dao *WorkerLogDAO) LatestPerWorker(ctx context.Context) ([]model.WorkerLog, error) {
	var logs []model.WorkerLog
	if err := dao.db.WithContext(ctx).Raw(`
WITH latest_log_ids AS (
	SELECT worker_id, MAX(id) AS id
	FROM worker_run_log
	GROUP BY worker_id
)
SELECT
	log.id,
	log.worker_id,
	log.request_id,
	log.status,
	log.error,
	log.duration_ms,
	log.created_at
FROM latest_log_ids
INNER JOIN worker_run_log AS log ON log.id = latest_log_ids.id
INNER JOIN worker ON worker.id = log.worker_id
ORDER BY log.created_at DESC, log.id DESC
`).Scan(&logs).Error; err != nil {
		return nil, err
	}
	return logs, nil
}

func (dao *WorkerLogDAO) SummaryBetween(ctx context.Context, since time.Time, before time.Time) (WorkerLogSummary, error) {
	var summary WorkerLogSummary
	err := dao.db.WithContext(ctx).Raw(`
SELECT
	COUNT(*) AS total,
	COALESCE(SUM(CASE WHEN COALESCE(log.error, '') = '' AND log.status >= 200 AND log.status < 400 THEN 1 ELSE 0 END), 0) AS success,
	COALESCE(SUM(CASE WHEN COALESCE(log.error, '') = '' AND log.status >= 200 AND log.status < 400 THEN 0 ELSE 1 END), 0) AS failed,
	COALESCE(SUM(log.duration_ms), 0) AS duration_total
FROM worker_run_log AS log
INNER JOIN worker ON worker.id = log.worker_id
WHERE log.created_at >= ? AND log.created_at < ?
`, since.UTC(), before.UTC()).Scan(&summary).Error
	return summary, err
}

func (dao *WorkerLogDAO) AggregateByWorkerBetween(ctx context.Context, since time.Time, before time.Time) ([]WorkerLogAggregate, error) {
	var aggregates []WorkerLogAggregate
	err := dao.db.WithContext(ctx).Raw(`
SELECT
	log.worker_id,
	COUNT(*) AS total,
	SUM(CASE WHEN COALESCE(log.error, '') = '' AND log.status >= 200 AND log.status < 400 THEN 1 ELSE 0 END) AS success,
	SUM(CASE WHEN COALESCE(log.error, '') = '' AND log.status >= 200 AND log.status < 400 THEN 0 ELSE 1 END) AS failed,
	SUM(log.duration_ms) AS duration_total
FROM worker_run_log AS log
INNER JOIN worker ON worker.id = log.worker_id
WHERE log.created_at >= ? AND log.created_at < ?
GROUP BY log.worker_id
`, since.UTC(), before.UTC()).Scan(&aggregates).Error
	return aggregates, err
}

func (dao *WorkerLogDAO) TrendBetween(ctx context.Context, workerID string, since time.Time, before time.Time) ([]WorkerLogTrendAggregate, error) {
	query := `
SELECT
	CAST(strftime('%s', log.created_at) AS INTEGER) / 3600 AS hour_unix,
	COUNT(*) AS total,
	SUM(CASE WHEN COALESCE(log.error, '') = '' AND log.status >= 200 AND log.status < 400 THEN 1 ELSE 0 END) AS success,
	SUM(CASE WHEN COALESCE(log.error, '') = '' AND log.status >= 200 AND log.status < 400 THEN 0 ELSE 1 END) AS failed,
	SUM(log.duration_ms) AS duration_total
FROM worker_run_log AS log
INNER JOIN worker ON worker.id = log.worker_id
WHERE log.created_at >= ? AND log.created_at < ?`
	args := []any{since.UTC(), before.UTC()}
	if workerID != "" {
		query += " AND log.worker_id = ?"
		args = append(args, workerID)
	}
	query += " GROUP BY hour_unix ORDER BY hour_unix ASC"

	var aggregates []WorkerLogTrendAggregate
	if err := dao.db.WithContext(ctx).Raw(query, args...).Scan(&aggregates).Error; err != nil {
		return nil, err
	}
	return aggregates, nil
}
