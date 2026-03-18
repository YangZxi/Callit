package model

import "time"

// WorkerLog 表示函数执行日志。
type WorkerLog struct {
	ID         int64     `json:"id" gorm:"column:id;primaryKey;autoIncrement"`
	WorkerID   string    `json:"worker_id" gorm:"column:worker_id;type:text;not null;index:idx_worker_run_log_worker_created,priority:1"`
	RequestID  string    `json:"request_id" gorm:"column:request_id;type:text;not null;index:idx_worker_run_log_request_id"`
	Status     int       `json:"status" gorm:"column:status"`
	Stdin      string    `json:"stdin" gorm:"column:stdin;type:text"`
	Stdout     string    `json:"stdout" gorm:"column:stdout;type:text"`
	Stderr     string    `json:"stderr" gorm:"column:stderr;type:text"`
	Result     string    `json:"result" gorm:"column:result;type:text"`
	Error      string    `json:"error" gorm:"column:error;type:text"`
	DurationMS int64     `json:"duration_ms" gorm:"column:duration_ms"`
	CreatedAt  time.Time `json:"created_at" gorm:"column:created_at;not null;index:idx_worker_run_log_worker_created,priority:2;autoCreateTime:false"`
}

func (WorkerLog) TableName() string {
	return "worker_run_log"
}
