package router

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

	wildcardBase := r.Match("/api/disabled")
	if !wildcardBase.Found || wildcardBase.Worker.ID != "f2" {
		t.Fatalf("通配基础路径应命中: %#v", wildcardBase)
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

	timeSlashMatch := r.Match("/time/?key=123")
	if !timeSlashMatch.Found || timeSlashMatch.Worker.ID != "exact-time" {
		t.Fatalf("/time/ 精确匹配失败: %#v", timeSlashMatch)
	}

	timeFullURLMatch := r.Match("https://test.com/time/?key=123")
	if !timeFullURLMatch.Found || timeFullURLMatch.Worker.ID != "exact-time" {
		t.Fatalf("完整 URL 精确匹配失败: %#v", timeFullURLMatch)
	}

	teaBaseMatch := r.Match("/tea")
	if !teaBaseMatch.Found || teaBaseMatch.Worker.ID != "wild-tea" {
		t.Fatalf("/tea 基础路径匹配失败: %#v", teaBaseMatch)
	}

	teaSlashMatch := r.Match("/tea/")
	if !teaSlashMatch.Found || teaSlashMatch.Worker.ID != "wild-tea" {
		t.Fatalf("/tea/ 通配匹配失败: %#v", teaSlashMatch)
	}

	teaMatch := r.Match("/tea/hot/get")
	if !teaMatch.Found || teaMatch.Worker.ID != "wild-tea" {
		t.Fatalf("/tea/* 通配匹配失败: %#v", teaMatch)
	}

	teaFullURLMatch := r.Match("https://c.xiaosm.cn/tea/hot/get?temp=1")
	if !teaFullURLMatch.Found || teaFullURLMatch.Worker.ID != "wild-tea" {
		t.Fatalf("完整 URL 通配匹配失败: %#v", teaFullURLMatch)
	}

	notMatchTeam := r.Match("/team")
	if notMatchTeam.Found {
		t.Fatalf("/team 不应命中 /tea/*: %#v", notMatchTeam)
	}

	notMatchTeamSlash := r.Match("/team/")
	if notMatchTeamSlash.Found {
		t.Fatalf("/team/ 不应命中 /tea/*: %#v", notMatchTeamSlash)
	}

	notMatchTeapot := r.Match("/teapot")
	if notMatchTeapot.Found {
		t.Fatalf("/teapot 不应命中 /tea/*: %#v", notMatchTeapot)
	}

	notMatch := r.Match("/team/hot/get")
	if notMatch.Found {
		t.Fatalf("非匹配前缀不应命中: %#v", notMatch)
	}
}
