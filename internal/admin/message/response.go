package message

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
