package model

import (
	"fmt"
	"strings"
	"time"
)

// Worker 表示可执行 Worker 元数据。
type Worker struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Runtime   string    `json:"runtime"`
	Route     string    `json:"route"`
	TimeoutMS int       `json:"timeout_ms"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Validate 用于校验函数配置。
func (f *Worker) Validate() error {
	if strings.TrimSpace(f.Name) == "" {
		return fmt.Errorf("name 不能为空")
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
	if !strings.Contains(route, "*") {
		return nil
	}
	if strings.Count(route, "*") != 1 || !strings.HasSuffix(route, "/*") {
		return fmt.Errorf("route 使用通配符时只支持结尾 /* 形式")
	}
	return nil
}
