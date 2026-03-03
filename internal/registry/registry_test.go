package registry

import (
	"testing"

	"callit/internal/model"
)

func TestRegistryMatch(t *testing.T) {
	r := New()
	r.Reload([]model.Worker{
		{
			ID:      "f1",
			Route:   "/api/demo",
			Enabled: true,
		},
		{
			ID:      "f2",
			Route:   "/api/disabled/*",
			Enabled: true,
		},
	})

	t.Log(r.wildcardRoutes)

	ok := r.Match("/api/demo")
	if !ok.Found || ok.Worker.ID != "f1" {
		t.Fatalf("路由匹配失败: %#v", ok)
	}

	notFound := r.Match("/api/not-exist")
	if notFound.Found {
		t.Fatalf("不存在路由匹配错误: %#v", notFound)
	}

	disabled := r.Match("/api/disabled")
	if disabled.Found {
		t.Fatalf("禁用函数不应可匹配: %#v", disabled)
	}
}

func TestRegistryMatchWildcardRoute(t *testing.T) {
	r := New()
	r.Reload([]model.Worker{
		{
			ID:      "exact-time",
			Route:   "/time",
			Enabled: true,
		},
		{
			ID:      "wild-tea",
			Route:   "/tea/*",
			Enabled: true,
		},
	})

	timeMatch := r.Match("/time")
	if !timeMatch.Found || timeMatch.Worker.ID != "exact-time" {
		t.Fatalf("/time 精确匹配失败: %#v", timeMatch)
	}

	teaMatch := r.Match("/tea/hot/get")
	if !teaMatch.Found || teaMatch.Worker.ID != "wild-tea" {
		t.Fatalf("/tea/* 通配匹配失败: %#v", teaMatch)
	}

	notMatch := r.Match("/team/hot/get")
	if notMatch.Found {
		t.Fatalf("非匹配前缀不应命中: %#v", notMatch)
	}
}
