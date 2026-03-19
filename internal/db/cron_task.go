package db

import (
	"context"
	"database/sql"
	"time"

	"callit/internal/model"

	"gorm.io/gorm"
)

type CronTaskWithWorker struct {
	Task   model.CronTask
	Worker model.Worker
}

type CronTaskDAO struct {
	db *gorm.DB
}

func (dao *CronTaskDAO) Create(ctx context.Context, task model.CronTask) (model.CronTask, error) {
	now := time.Now().UTC()
	task.CreatedAt = now
	task.UpdatedAt = now
	if err := dao.db.WithContext(ctx).Create(&task).Error; err != nil {
		return model.CronTask{}, err
	}
	return task, nil
}

func (dao *CronTaskDAO) ListByWorkerID(ctx context.Context, workerID string) ([]model.CronTask, error) {
	var tasks []model.CronTask
	if err := dao.db.WithContext(ctx).
		Where("worker_id = ?", workerID).
		Order("id ASC").
		Find(&tasks).Error; err != nil {
		return nil, err
	}
	return tasks, nil
}

func (dao *CronTaskDAO) GetByID(ctx context.Context, id int64, workerID string) (model.CronTask, error) {
	var task model.CronTask
	if err := dao.db.WithContext(ctx).
		Where("id = ? AND worker_id = ?", id, workerID).
		First(&task).Error; err != nil {
		return model.CronTask{}, notFoundError(err)
	}
	return task, nil
}

func (dao *CronTaskDAO) Update(ctx context.Context, task model.CronTask) (model.CronTask, error) {
	result := dao.db.WithContext(ctx).
		Model(&model.CronTask{}).
		Where("id = ? AND worker_id = ?", task.ID, task.WorkerID).
		Updates(map[string]any{
			"cron":       task.Cron,
			"updated_at": time.Now().UTC(),
		})
	if result.Error != nil {
		return model.CronTask{}, result.Error
	}
	if result.RowsAffected == 0 {
		return model.CronTask{}, sql.ErrNoRows
	}
	return dao.GetByID(ctx, task.ID, task.WorkerID)
}

func (dao *CronTaskDAO) Delete(ctx context.Context, id int64, workerID string) error {
	result := dao.db.WithContext(ctx).
		Where("id = ? AND worker_id = ?", id, workerID).
		Delete(&model.CronTask{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (dao *CronTaskDAO) DeleteByWorkerID(ctx context.Context, workerID string) error {
	return dao.db.WithContext(ctx).
		Where("worker_id = ?", workerID).
		Delete(&model.CronTask{}).Error
}

func (dao *CronTaskDAO) ListEnabledWithWorkers(ctx context.Context) ([]CronTaskWithWorker, error) {
	type row struct {
		TaskID        int64
		TaskWorkerID  string
		TaskCron      string
		TaskCreatedAt time.Time
		TaskUpdatedAt time.Time
		WorkerID      string
		WorkerName    string
		WorkerRuntime string
		WorkerRoute   string
		WorkerTimeout int
		WorkerEnabled bool
		WorkerCreated time.Time
		WorkerUpdated time.Time
	}

	var rows []row
	if err := dao.db.WithContext(ctx).
		Table("cron_task").
		Select([]string{
			"cron_task.id AS task_id",
			"cron_task.worker_id AS task_worker_id",
			"cron_task.cron AS task_cron",
			"cron_task.created_at AS task_created_at",
			"cron_task.updated_at AS task_updated_at",
			"worker.id AS worker_id",
			"worker.name AS worker_name",
			"worker.runtime AS worker_runtime",
			"worker.route AS worker_route",
			"worker.timeout_ms AS worker_timeout",
			"worker.enabled AS worker_enabled",
			"worker.created_at AS worker_created",
			"worker.updated_at AS worker_updated",
		}).
		Joins("JOIN worker ON worker.id = cron_task.worker_id").
		Where("worker.enabled = ?", true).
		Order("cron_task.id ASC").
		Scan(&rows).Error; err != nil {
		return nil, err
	}

	result := make([]CronTaskWithWorker, 0, len(rows))
	for _, item := range rows {
		result = append(result, CronTaskWithWorker{
			Task: model.CronTask{
				ID:        item.TaskID,
				WorkerID:  item.TaskWorkerID,
				Cron:      item.TaskCron,
				CreatedAt: item.TaskCreatedAt,
				UpdatedAt: item.TaskUpdatedAt,
			},
			Worker: model.Worker{
				ID:        item.WorkerID,
				Name:      item.WorkerName,
				Runtime:   item.WorkerRuntime,
				Route:     item.WorkerRoute,
				TimeoutMS: item.WorkerTimeout,
				Enabled:   item.WorkerEnabled,
				CreatedAt: item.WorkerCreated,
				UpdatedAt: item.WorkerUpdated,
			},
		})
	}
	return result, nil
}
