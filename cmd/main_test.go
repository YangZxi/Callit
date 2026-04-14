package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"callit/internal/config"
)

func TestServerRouteHandlerUsesLatestMCPEnableConfig(t *testing.T) {
	cfg := &config.Config{
		AdminPrefix: "/admin",
		AppConfig: config.AppConfig{
			MCP_Enable: false,
		},
	}

	adminHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Handler", "admin")
		w.WriteHeader(http.StatusOK)
	})
	mcpHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Handler", "mcp")
		w.WriteHeader(http.StatusOK)
	})
	routerHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Handler", "router")
		w.WriteHeader(http.StatusOK)
	})

	handler := serverRouteHandler(adminHandler, mcpHandler, routerHandler, cfg)

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	handler.ServeHTTP(recorder, req)
	if recorder.Header().Get("X-Handler") != "router" {
		t.Fatalf("MCP 关闭时不应转发到 mcp，实际命中 %q", recorder.Header().Get("X-Handler"))
	}

	cfg.AppConfig.MCP_Enable = true

	recorder = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/mcp", nil)
	handler.ServeHTTP(recorder, req)
	if recorder.Header().Get("X-Handler") != "mcp" {
		t.Fatalf("MCP 开启后应直接转发到 mcp，实际命中 %q", recorder.Header().Get("X-Handler"))
	}
}
