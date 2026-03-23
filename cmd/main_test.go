package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestServerRouteHandlerRoutesMCPPathWhenEnabled(t *testing.T) {
	adminHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})
	mcpHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	})
	routerHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := serverRouteHandler(adminHandler, mcpHandler, routerHandler, "/admin", true)

	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusAccepted {
		t.Fatalf("/mcp 启用时应路由到 MCP Handler，实际状态码为 %d", resp.Code)
	}
}

func TestServerRouteHandlerRoutesMCPPathToRouterWhenDisabled(t *testing.T) {
	adminHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})
	routerHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := serverRouteHandler(adminHandler, nil, routerHandler, "/admin", false)

	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("/mcp 关闭时应回落到 Router Handler，实际状态码为 %d", resp.Code)
	}
}
