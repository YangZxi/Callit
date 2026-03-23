package db

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"callit/internal/model"

	"gorm.io/gorm"
)

type WorkerDAO struct {
	db *gorm.DB
}

func (dao *WorkerDAO) Create(ctx context.Context, worker model.Worker) (model.Worker, error) {
	if worker.TimeoutMS <= 0 {
		worker.TimeoutMS = 5000
	}
	now := time.Now().UTC()
	worker.CreatedAt = now
	worker.UpdatedAt = now
	if err := dao.db.WithContext(ctx).Model(&model.Worker{}).Create(map[string]any{
		"id":         worker.ID,
		"name":       worker.Name,
		"runtime":    worker.Runtime,
		"route":      worker.Route,
		"timeout_ms": worker.TimeoutMS,
		"enabled":    worker.Enabled,
		"created_at": worker.CreatedAt,
		"updated_at": worker.UpdatedAt,
	}).Error; err != nil {
		return model.Worker{}, err
	}
	return worker, nil
}

func (dao *WorkerDAO) List(ctx context.Context, keyword string) ([]model.Worker, error) {
	var workers []model.Worker
	query := dao.db.WithContext(ctx).Model(&model.Worker{})
	if trimmedKeyword := strings.TrimSpace(keyword); trimmedKeyword != "" {
		likeKeyword := "%" + strings.ToLower(trimmedKeyword) + "%"
		query = query.Where("LOWER(name) LIKE ?", likeKeyword)
	}
	if err := query.
		Order("created_at DESC").
		Find(&workers).Error; err != nil {
		return nil, err
	}
	return workers, nil
}

func (dao *WorkerDAO) ListEnabled(ctx context.Context) ([]model.Worker, error) {
	var workers []model.Worker
	if err := dao.db.WithContext(ctx).
		Where("enabled = ?", true).
		Order("created_at DESC").
		Find(&workers).Error; err != nil {
		return nil, err
	}
	return workers, nil
}

func (dao *WorkerDAO) GetByID(ctx context.Context, id string) (model.Worker, error) {
	var worker model.Worker
	if err := dao.db.WithContext(ctx).Where("id = ?", id).First(&worker).Error; err != nil {
		return model.Worker{}, notFoundError(err)
	}
	return worker, nil
}

func (dao *WorkerDAO) Update(ctx context.Context, worker model.Worker) (model.Worker, error) {
	if worker.TimeoutMS <= 0 {
		worker.TimeoutMS = 5000
	}
	now := time.Now().UTC()
	result := dao.db.WithContext(ctx).
		Model(&model.Worker{}).
		Where("id = ?", worker.ID).
		Updates(map[string]any{
			"name":       worker.Name,
			"runtime":    worker.Runtime,
			"route":      worker.Route,
			"timeout_ms": worker.TimeoutMS,
			"enabled":    worker.Enabled,
			"updated_at": now,
		})
	if result.Error != nil {
		return model.Worker{}, result.Error
	}
	if result.RowsAffected == 0 {
		return model.Worker{}, sql.ErrNoRows
	}
	return dao.GetByID(ctx, worker.ID)
}

func (dao *WorkerDAO) Delete(ctx context.Context, id string) error {
	result := dao.db.WithContext(ctx).Where("id = ?", id).Delete(&model.Worker{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (dao *WorkerDAO) SetEnabled(ctx context.Context, id string, enabled bool) (model.Worker, error) {
	result := dao.db.WithContext(ctx).
		Model(&model.Worker{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"enabled":    enabled,
			"updated_at": time.Now().UTC(),
		})
	if result.Error != nil {
		return model.Worker{}, result.Error
	}
	if result.RowsAffected == 0 {
		return model.Worker{}, sql.ErrNoRows
	}
	return dao.GetByID(ctx, id)
}
