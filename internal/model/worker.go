package model

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

type WorkerEnv []string

func (env WorkerEnv) Value() (driver.Value, error) {
	normalized := sanitizeWorkerEnvEntries(env)
	raw, err := json.Marshal([]string(normalized))
	if err != nil {
		return nil, fmt.Errorf("序列化 Worker 环境变量失败: %w", err)
	}
	return string(raw), nil
}

func (env *WorkerEnv) Scan(src any) error {
	if env == nil {
		return fmt.Errorf("WorkerEnv Scan 目标不能为空")
	}
	switch value := src.(type) {
	case nil:
		*env = WorkerEnv{}
		return nil
	case string:
		return env.scanString(value)
	case []byte:
		return env.scanString(string(value))
	default:
		return fmt.Errorf("不支持的 WorkerEnv 数据类型: %T", src)
	}
}

func (env *WorkerEnv) scanString(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		*env = WorkerEnv{}
		return nil
	}

	var items []string
	if err := json.Unmarshal([]byte(raw), &items); err == nil {
		*env = sanitizeWorkerEnvEntries(items)
		return nil
	}

	*env = parseDeprecatedWorkerEnvString(raw)
	return nil
}

func sanitizeWorkerEnvEntries(entries []string) WorkerEnv {
	normalized := make(WorkerEnv, 0, len(entries))
	for _, item := range entries {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		normalized = append(normalized, trimmed)
	}
	return normalized
}

// Deprecated: 仅用于兼容历史上以普通字符串存储的 Worker 环境变量；待旧数据迁移完成后应删除。
func parseDeprecatedWorkerEnvString(raw string) WorkerEnv {
	raw = strings.NewReplacer("\r\n", "\n", "\r", "\n").Replace(raw)
	splitter := func(r rune) bool {
		return r == '\n' || r == ';'
	}
	return sanitizeWorkerEnvEntries(strings.FieldsFunc(raw, splitter))
}

// Worker 表示可执行 Worker 元数据。
type Worker struct {
	ID          string    `json:"id" gorm:"column:id;primaryKey;type:text"`
	Name        string    `json:"name" gorm:"column:name;type:text;not null"`
	Description string    `json:"description" gorm:"column:description;type:text;not null;default:''"`
	Runtime     string    `json:"runtime" gorm:"column:runtime;type:text;not null"`
	Route       string    `json:"route" gorm:"column:route;type:text;not null;uniqueIndex"`
	TimeoutMS   int       `json:"timeout_ms" gorm:"column:timeout_ms;not null;default:5000"`
	Env         WorkerEnv `json:"env" gorm:"column:env;type:text;not null;default:''"`
	Enabled     bool      `json:"enabled" gorm:"column:enabled;not null;default:true"`
	UpdatedAt   time.Time `json:"updated_at" gorm:"column:updated_at;not null;autoUpdateTime:false"`
	CreatedAt   time.Time `json:"created_at" gorm:"column:created_at;not null;autoCreateTime:false"`
}

func (Worker) TableName() string {
	return "worker"
}

// Validate 用于校验函数配置。
func (f *Worker) Validate() error {
	if strings.TrimSpace(f.Name) == "" {
		return fmt.Errorf("name 不能为空")
	}
	if utf8.RuneCountInString(f.Description) > 200 {
		return fmt.Errorf("description 不能超过 200 字符")
	}
	if f.Runtime != "python" && f.Runtime != "node" {
		return fmt.Errorf("runtime 仅支持 python 或 node")
	}
	if err := ValidateRoute(f.Route); err != nil {
		return err
	}
	if f.TimeoutMS <= 0 {
		return fmt.Errorf("timeout_ms 必须大于 0")
	}
	return nil
}

// ValidateRoute 校验 Worker 路由规则。
func ValidateRoute(route string) error {
	if !strings.HasPrefix(route, "/") {
		return fmt.Errorf("route 必须以 / 开头")
	}
	if route == "/*" {
		return fmt.Errorf("route 不能使用泛根路径 /*")
	}
	if !strings.Contains(route, "*") {
		return nil
	}
	if strings.Count(route, "*") != 1 || !strings.HasSuffix(route, "/*") {
		return fmt.Errorf("route 使用通配符时只支持结尾 /* 形式")
	}
	return nil
}
