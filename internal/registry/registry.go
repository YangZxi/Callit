package registry

import (
	"net/url"
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

func normalizeRoutePath(route string) string {
	if parsed, err := url.Parse(route); err == nil && parsed.Path != "" {
		route = parsed.Path
	} else if index := strings.Index(route, "?"); index >= 0 {
		route = route[:index]
	}

	if route == "" || route == "/" {
		return "/"
	}

	normalized := strings.TrimRight(route, "/")
	if normalized == "" {
		return "/"
	}
	return normalized
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
	for _, worker := range functions {
		if !worker.Enabled {
			continue
		}

		target := nextExact
		routeKey := normalizeRoutePath(worker.Route)
		if strings.HasSuffix(worker.Route, "/*") {
			target = nextWildcard
			// 只保留前缀部分，示例: /tea/* => /tea
			routeKey = normalizeRoutePath(strings.TrimSuffix(worker.Route, "/*"))
		}
		target[routeKey] = worker
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

	normalizedRoute := normalizeRoutePath(route)

	if worker, ok := r.exactRoutes[normalizedRoute]; ok {
		return RouterInfo{Worker: worker, Found: true}
	}

	for _, item := range r.wildcardRoutes {
		if normalizedRoute == item.prefix || strings.HasPrefix(normalizedRoute, item.prefix+"/") {
			return RouterInfo{Worker: item.worker, Found: true}
		}
	}

	return RouterInfo{Found: false}
}
