package model

import "time"

// WorkerLog 表示函数执行日志。
type WorkerLog struct {
	ID         string    `json:"id"`
	WorkerID   string    `json:"worker_id"`
	RequestID  string    `json:"request_id"`
	Status     int       `json:"status"`
	Stdout     string    `json:"stdout"`
	Stderr     string    `json:"stderr"`
	Error      string    `json:"error"`
	DurationMS int64     `json:"duration_ms"`
	CreatedAt  time.Time `json:"created_at"`
}
