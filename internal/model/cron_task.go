package model

import "time"

// CronTask 表示 Worker 的定时任务配置。
type CronTask struct {
	ID        int64     `json:"id,string" gorm:"column:id;primaryKey;autoIncrement:false"`
	WorkerID  string    `json:"worker_id" gorm:"column:worker_id;type:text;not null;index:idx_cron_task_worker_id"`
	Cron      string    `json:"cron" gorm:"column:cron;type:text;not null"`
	CreatedAt time.Time `json:"created_at" gorm:"column:created_at;not null;autoCreateTime:false"`
	UpdatedAt time.Time `json:"updated_at" gorm:"column:updated_at;not null;autoUpdateTime:false"`
}

func (CronTask) TableName() string {
	return "cron_task"
}
