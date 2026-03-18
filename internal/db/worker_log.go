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

func (dao *WorkerLogDAO) Insert(ctx context.Context, log model.WorkerLog) error {
	if log.CreatedAt.IsZero() {
		log.CreatedAt = time.Now().UTC()
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
