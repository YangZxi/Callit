package model

import "time"

// WorkerLog 表示函数执行日志。
type WorkerLog struct {
	ID         int64     `json:"id,string" gorm:"column:id;primaryKey;autoIncrement"`
	WorkerID   string    `json:"worker_id" gorm:"column:worker_id;type:text;not null;index:idx_worker_run_log_worker_created,priority:1"`
	RequestID  string    `json:"request_id" gorm:"column:request_id;type:text;not null;index:idx_worker_run_log_request_id"`
	Trigger    string    `json:"trigger" gorm:"column:trigger;type:text;not null;default:http"`
	Status     int       `json:"status" gorm:"column:status"`
	Stdin      string    `json:"stdin" gorm:"column:stdin;type:text"`
	Stdout     string    `json:"stdout" gorm:"column:stdout;type:text"`
	Stderr     string    `json:"stderr" gorm:"column:stderr;type:text"`
	ExecLog    string    `json:"exec_log" gorm:"column:exec_log;type:text"`
	Result     string    `json:"result" gorm:"column:result;type:text"`
	Error      string    `json:"error" gorm:"column:error;type:text"`
	DurationMS int64     `json:"duration_ms" gorm:"column:duration_ms"`
	CreatedAt  time.Time `json:"created_at" gorm:"column:created_at;not null;index:idx_worker_run_log_worker_created,priority:2;autoCreateTime:false"`
}

func (WorkerLog) TableName() string {
	return "worker_run_log"
}

func (log WorkerLog) IsSuccess() bool {
	return log.Error == "" && log.Status >= 200 && log.Status < 500
}

func (log WorkerLog) IsServerError() bool {
	return log.Error != "" || (log.Status >= 500 && log.Status < 600)
}
