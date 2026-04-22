package message

import "time"

// DependencyInfo 表示依赖项信息。
type DependencyInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// DependencyLogEvent 表示依赖管理的 SSE 日志事件。
type DependencyLogEvent struct {
	Stream string `json:"stream"`
	Text   string `json:"text"`
}

// AdminConfigItem 表示后台配置项展示结构。
type AdminConfigItem struct {
	Key    string  `json:"key"`
	Value  string  `json:"value"`
	Source string  `json:"source"`
	DB     *string `json:"db,omitempty"`
}

// DashboardMetricsResponse 表示 Dashboard 概览统计响应。
type DashboardMetricsResponse struct {
	GeneratedAt       time.Time                   `json:"generated_at"`
	Workers           DashboardWorkerCounts       `json:"workers"`
	Summary           DashboardSummary            `json:"summary"`
	LastFailedWorkers []DashboardLastFailedWorker `json:"last_failed_workers"`
	WorkerRankings    DashboardWorkerRankings     `json:"worker_rankings"`
}

// DashboardWorkerCounts 表示 Worker 数量统计。
type DashboardWorkerCounts struct {
	Total    int `json:"total"`
	Enabled  int `json:"enabled"`
	Disabled int `json:"disabled"`
}

// DashboardSummary 表示 Dashboard 顶部核心指标。
type DashboardSummary struct {
	SuccessRate24h         *float64 `json:"success_rate_24h"`
	SuccessRate6h          *float64 `json:"success_rate_6h"`
	AvgDurationMS24h       *int64   `json:"avg_duration_ms_24h"`
	TotalCalls24h          int      `json:"total_calls_24h"`
	SuccessCalls24h        int      `json:"success_calls_24h"`
	FailedCalls24h         int      `json:"failed_calls_24h"`
	TotalCalls6h           int      `json:"total_calls_6h"`
	FailedCalls6h          int      `json:"failed_calls_6h"`
	LastFailedWorkersCount int      `json:"last_failed_workers_count"`
}

// DashboardLastFailedWorker 表示最后一次调用处于失败状态的 Worker。
type DashboardLastFailedWorker struct {
	WorkerID       string    `json:"worker_id"`
	WorkerName     string    `json:"worker_name"`
	LastLogID      string    `json:"last_log_id"`
	LastRequestID  string    `json:"last_request_id"`
	LastStatus     int       `json:"last_status"`
	LastDurationMS int64     `json:"last_duration_ms"`
	LastFailedAt   time.Time `json:"last_failed_at"`
	LastError      string    `json:"last_error"`
}

// DashboardWorkerRankings 表示 Worker 健康排行。
type DashboardWorkerRankings struct {
	Slowest       []DashboardSlowWorkerRankItem     `json:"slowest"`
	LeastReliable []DashboardReliableWorkerRankItem `json:"least_reliable"`
}

// DashboardSlowWorkerRankItem 表示平均耗时最高的 Worker。
type DashboardSlowWorkerRankItem struct {
	WorkerID      string   `json:"worker_id"`
	WorkerName    string   `json:"worker_name"`
	Calls         int      `json:"calls"`
	SuccessRate   *float64 `json:"success_rate"`
	AvgDurationMS int64    `json:"avg_duration_ms"`
}

// DashboardReliableWorkerRankItem 表示成功率最低的 Worker。
type DashboardReliableWorkerRankItem struct {
	WorkerID    string   `json:"worker_id"`
	WorkerName  string   `json:"worker_name"`
	Calls       int      `json:"calls"`
	SuccessRate *float64 `json:"success_rate"`
	Failed      int      `json:"failed"`
}

// DashboardWorkerTrendPoint 表示 Worker 调用趋势图中的一个时间桶。
type DashboardWorkerTrendPoint struct {
	Time          time.Time `json:"time"`
	Total         int       `json:"total"`
	Success       int       `json:"success"`
	Failed        int       `json:"failed"`
	SuccessRate   *float64  `json:"success_rate"`
	AvgDurationMS *int64    `json:"avg_duration_ms"`
}
