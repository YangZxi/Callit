package model

const (
	WorkerTriggerHTTP = "http"
	WorkerTriggerCron = "cron"
)

// WorkerInput 是传给脚本的函数执行上下文。
type WorkerInput struct {
	// Request 表示当前 HTTP 请求的结构化信息。
	Request WorkerRequest `json:"request"`
	// Event 表示本次调用的运行时事件上下文。
	Event WorkerEvent `json:"event"`
}

// WorkerEvent 描述当前执行事件上下文。
type WorkerEvent struct {
	// RequestID 是本次请求的唯一追踪标识。
	RequestID string `json:"request_id"`
	// Trigger 是本次运行的触发类型。
	Trigger string `json:"trigger"`
	// Runtime 是当前 Worker 运行时类型（如 node/python）。
	Runtime string `json:"runtime"`
	// WorkerID 是被调用 Worker 的唯一标识。
	WorkerID string `json:"worker_id"`
	// Route 是命中的 Worker 路由规则。
	Route string `json:"route"`
}

// WorkerRequest 描述 HTTP 请求信息。
type WorkerRequest struct {
	// Method HTTP 请求方法。
	Method string `json:"method,omitempty"`
	// URI 供 Worker 使用的相对请求地址。
	URI string `json:"uri,omitempty"`
	// URL 完整请求地址。
	URL string `json:"url,omitempty"`
	// Params 查询参数，重复键以后出现的值覆盖前值。
	Params map[string]string `json:"params,omitempty"`
	// Headers 请求头键值对。
	Headers map[string]string `json:"headers,omitempty"`
	// Body 原始请求体字符串（multipart/form-data 时固定为空，避免传递过大内容）。
	Body string `json:"body,omitempty"`
	// JSON 按请求类型解析后的结构化请求体。
	JSON any `json:"json,omitempty"`
}

// WorkerOutput 是脚本 stdout 结构化响应。
type WorkerOutput struct {
	// Status Worker 希望返回的 HTTP 状态码，默认 200。
	Status *int `json:"status,omitempty"`
	// Headers Worker 希望附加的响应头。
	Headers map[string]string `json:"headers,omitempty"`
	// File Worker 目录下的相对文件路径，用于返回文件内容。
	File string `json:"file,omitempty"`
	// Body 业务响应体，可为对象、数组或字符串。
	Body any `json:"body"`
}
