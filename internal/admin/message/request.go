package message

// CreateWorkerRequest 表示创建或更新 Worker 的请求体。
type CreateWorkerRequest struct {
	Name      string `json:"name"`
	Runtime   string `json:"runtime"`
	Route     string `json:"route"`
	TimeoutMS int    `json:"timeout_ms"`
	Enabled   *bool  `json:"enabled"`
}

// WorkerIDRequest 表示仅包含 Worker ID 的请求体。
type WorkerIDRequest struct {
	ID string `json:"id"`
}

// LoginRequest 表示管理员登录请求体。
type LoginRequest struct {
	Token string `json:"token"`
}

// SaveFileRequest 表示保存文件内容请求体。
type SaveFileRequest struct {
	Filename string `json:"filename"`
	Content  string `json:"content"`
}

// DeleteFileRequest 表示删除文件请求体。
type DeleteFileRequest struct {
	Filename string `json:"filename"`
}

// RenameFileRequest 表示重命名文件请求体。
type RenameFileRequest struct {
	Filename    string `json:"filename"`
	NewFilename string `json:"new_filename"`
}

// DependencyManageRequest 表示依赖管理请求体。
type DependencyManageRequest struct {
	Runtime string `json:"runtime"`
	Action  string `json:"action"`
	Package string `json:"package"`
}

// AdminUpsertConfigRequest 表示后台配置更新请求体。
type AdminUpsertConfigRequest struct {
	AppConfig map[string]*string `json:"app_config"`
}

// ChatStreamRequest 表示聊天流式请求体。
type ChatStreamRequest struct {
	Mode         string `json:"mode"`
	Message      string `json:"message"`
	HistoryLimit int    `json:"history_limit"`
}
