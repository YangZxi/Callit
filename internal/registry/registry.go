package registry

import (
	"sort"
	"strings"
	"sync"

	"callit/internal/model"
)

// RouterInfo 是路由匹配结果。
type RouterInfo struct {
	Worker model.Worker
	Found  bool
}

// Registry 保存启用函数的内存索引。
type Registry struct {
	mu             sync.RWMutex
	exactRoutes    map[string]model.Worker
	wildcardRoutes []wildcardRoute
}

type wildcardRoute struct {
	prefix string
	worker model.Worker
}

// New 创建空注册表。
func New() *Registry {
	return &Registry{
		exactRoutes: make(map[string]model.Worker),
	}
}

// Reload 全量重建路由索引。
func (r *Registry) Reload(functions []model.Worker) {
	nextExact := make(map[string]model.Worker, len(functions))
	nextWildcard := make(map[string]model.Worker)
	for _, fn := range functions {
		if !fn.Enabled {
			continue
		}

		target := nextExact
		routeKey := fn.Route
		if strings.HasSuffix(fn.Route, "/*") {
			target = nextWildcard
			// 只保留前缀部分，示例: /tea/* => /tea/
			routeKey = strings.TrimSuffix(fn.Route, "*")
		}
		target[routeKey] = fn
	}

	wildcards := make([]wildcardRoute, 0, len(nextWildcard))
	for prefix, worker := range nextWildcard {
		wildcards = append(wildcards, wildcardRoute{
			prefix: prefix,
			worker: worker,
		})
	}
	// 前缀越长越优先，避免短前缀吞掉更具体的路由。
	sort.Slice(wildcards, func(i, j int) bool {
		return len(wildcards[i].prefix) > len(wildcards[j].prefix)
	})

	r.mu.Lock()
	r.exactRoutes = nextExact
	r.wildcardRoutes = wildcards
	r.mu.Unlock()
}

// Match 按路径匹配函数。
func (r *Registry) Match(route string) RouterInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if fn, ok := r.exactRoutes[route]; ok {
		return RouterInfo{Worker: fn, Found: true}
	}

	for _, item := range r.wildcardRoutes {
		if !strings.HasPrefix(route, item.prefix) {
			continue
		}
		return RouterInfo{Worker: item.worker, Found: true}
	}

	return RouterInfo{Found: false}
}
