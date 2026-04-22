package db

import (
	"context"
	"time"

	"callit/internal/model"

	"gorm.io/gorm"
)

type WorkerLogDAO struct {
	db *gorm.DB
}

// WorkerLogAggregate 表示一个 Worker 在指定时间窗口内的运行聚合。
type WorkerLogAggregate struct {
	WorkerID      string
	Total         int
	Success       int
	Failed        int
	AvgDurationMS int64
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

func (dao *WorkerLogDAO) ListSince(ctx context.Context, workerID string, since time.Time) ([]model.WorkerLog, error) {
	query := dao.db.WithContext(ctx).
		Table(model.WorkerLog{}.TableName()+" AS log").
		Select("log.*").
		Joins("INNER JOIN worker ON worker.id = log.worker_id").
		Where("log.created_at >= ?", since.UTC()).
		Order("log.created_at ASC, log.id ASC")

	if workerID != "" {
		query = query.Where("log.worker_id = ?", workerID)
	}

	var logs []model.WorkerLog
	if err := query.Find(&logs).Error; err != nil {
		return nil, err
	}
	return logs, nil
}

func (dao *WorkerLogDAO) LatestPerWorker(ctx context.Context) ([]model.WorkerLog, error) {
	var logs []model.WorkerLog
	if err := dao.db.WithContext(ctx).Raw(`
SELECT *
FROM (
	SELECT
		worker_run_log.*,
		ROW_NUMBER() OVER (
			PARTITION BY worker_run_log.worker_id
			ORDER BY worker_run_log.created_at DESC, worker_run_log.id DESC
		) AS row_number
	FROM worker_run_log
	INNER JOIN worker ON worker.id = worker_run_log.worker_id
) AS ranked_logs
WHERE row_number = 1
ORDER BY created_at DESC, id DESC
`).Scan(&logs).Error; err != nil {
		return nil, err
	}
	return logs, nil
}

func (dao *WorkerLogDAO) AggregateByWorkerSince(ctx context.Context, since time.Time) ([]WorkerLogAggregate, error) {
	logs, err := dao.ListSince(ctx, "", since)
	if err != nil {
		return nil, err
	}

	itemsByWorker := make(map[string]*WorkerLogAggregate)
	durationSums := make(map[string]int64)
	for _, log := range logs {
		item := itemsByWorker[log.WorkerID]
		if item == nil {
			item = &WorkerLogAggregate{WorkerID: log.WorkerID}
			itemsByWorker[log.WorkerID] = item
		}

		item.Total++
		durationSums[log.WorkerID] += log.DurationMS
		if log.IsSuccess() {
			item.Success++
		} else {
			item.Failed++
		}
	}

	items := make([]WorkerLogAggregate, 0, len(itemsByWorker))
	for workerID, item := range itemsByWorker {
		if item.Total > 0 {
			item.AvgDurationMS = durationSums[workerID] / int64(item.Total)
		}
		items = append(items, *item)
	}
	return items, nil
}
