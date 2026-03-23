package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"callit/internal/config"
	"callit/internal/db"
	"callit/internal/model"
	"callit/internal/router"
)

func openMCPTestStore(t *testing.T) *db.Store {
	t.Helper()

	store, err := db.Open(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatalf("打开测试数据库失败: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("关闭测试数据库失败: %v", err)
		}
	})
	return store
}

func createMCPTestWorker(t *testing.T, store *db.Store, worker model.Worker) {
	t.Helper()

	if _, err := store.Worker.Create(context.Background(), worker); err != nil {
		t.Fatalf("创建测试 Worker 失败: %v", err)
	}
}

func doMCPRequest(t *testing.T, handler http.Handler, token string, body any) *httptest.ResponseRecorder {
	t.Helper()

	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("序列化请求失败: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	return resp
}

func TestNewHandlerRejectsUnauthorizedRequest(t *testing.T) {
	store := openMCPTestStore(t)
	cfg := config.Config{
		DataDir: filepath.Join(t.TempDir(), "data"),
		AppConfig: config.AppConfig{
			MCP_Enable: true,
			MCP_Token:  "mcp-token",
		},
	}
	handler := NewHandler(store, router.New(), nil, &cfg)

	resp := doMCPRequest(t, handler, "wrong-token", map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": "2025-06-18",
			"capabilities":    map[string]any{},
			"clientInfo": map[string]any{
				"name":    "test-client",
				"version": "1.0.0",
			},
		},
	})

	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("错误 token 应返回 401，实际为 %d，body=%s", resp.Code, resp.Body.String())
	}
}

func TestNewHandlerSupportsInitializeAndToolsCall(t *testing.T) {
	store := openMCPTestStore(t)
	createMCPTestWorker(t, store, model.Worker{
		ID:        "worker-mcp-search",
		Name:      "支付 Worker",
		Runtime:   "python",
		Route:     "/pay",
		TimeoutMS: 5000,
		Enabled:   true,
	})

	cfg := config.Config{
		DataDir: filepath.Join(t.TempDir(), "data"),
		AppConfig: config.AppConfig{
			MCP_Enable: true,
			MCP_Token:  "mcp-token",
		},
	}
	handler := NewHandler(store, router.New(), nil, &cfg)

	initializeResp := doMCPRequest(t, handler, "mcp-token", map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": "2025-06-18",
			"capabilities":    map[string]any{},
			"clientInfo": map[string]any{
				"name":    "test-client",
				"version": "1.0.0",
			},
		},
	})
	if initializeResp.Code != http.StatusOK {
		t.Fatalf("initialize 应返回 200，实际为 %d，body=%s", initializeResp.Code, initializeResp.Body.String())
	}

	toolsResp := doMCPRequest(t, handler, "mcp-token", map[string]any{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "tools/call",
		"params": map[string]any{
			"name": "search_workers",
			"arguments": map[string]any{
				"keyword": "支付",
			},
		},
	})
	if toolsResp.Code != http.StatusOK {
		t.Fatalf("tools/call 应返回 200，实际为 %d，body=%s", toolsResp.Code, toolsResp.Body.String())
	}

	var body struct {
		Result struct {
			StructuredContent struct {
				Workers []model.Worker `json:"workers"`
			} `json:"structuredContent"`
		} `json:"result"`
	}
	if err := json.Unmarshal(toolsResp.Body.Bytes(), &body); err != nil {
		t.Fatalf("解析 tools/call 响应失败: %v", err)
	}
	if len(body.Result.StructuredContent.Workers) != 1 {
		t.Fatalf("search_workers 应返回 1 个 worker，实际为 %d", len(body.Result.StructuredContent.Workers))
	}
	if body.Result.StructuredContent.Workers[0].ID != "worker-mcp-search" {
		t.Fatalf("search_workers 返回错误 worker: %#v", body.Result.StructuredContent.Workers[0])
	}
}

func TestNewHandlerReturnsNotFoundWhenDisabled(t *testing.T) {
	store := openMCPTestStore(t)
	cfg := config.Config{
		DataDir: filepath.Join(t.TempDir(), "data"),
		AppConfig: config.AppConfig{
			MCP_Enable: false,
			MCP_Token:  "mcp-token",
		},
	}
	handler := NewHandler(store, router.New(), nil, &cfg)

	resp := doMCPRequest(t, handler, "mcp-token", map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
	})
	if resp.Code != http.StatusNotFound {
		t.Fatalf("MCP 关闭时应返回 404，实际为 %d，body=%s", resp.Code, resp.Body.String())
	}
}

func TestCreateAndUpdateWorkerTools(t *testing.T) {
	store := openMCPTestStore(t)
	dataDir := filepath.Join(t.TempDir(), "data")
	cfg := config.Config{
		DataDir: dataDir,
		AppConfig: config.AppConfig{
			MCP_Enable: true,
			MCP_Token:  "mcp-token",
		},
	}
	handler := NewHandler(store, router.New(), nil, &cfg)

	createResp := doMCPRequest(t, handler, "mcp-token", map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params": map[string]any{
			"name": "create_worker",
			"arguments": map[string]any{
				"name":       "订单 Worker",
				"runtime":    "python",
				"route":      "/orders/*",
				"timeout_ms": 4000,
				"enabled":    true,
			},
		},
	})
	if createResp.Code != http.StatusOK {
		t.Fatalf("create_worker 应返回 200，实际为 %d，body=%s", createResp.Code, createResp.Body.String())
	}

	var createBody struct {
		Result struct {
			StructuredContent struct {
				Worker model.Worker `json:"worker"`
			} `json:"structuredContent"`
		} `json:"result"`
	}
	if err := json.Unmarshal(createResp.Body.Bytes(), &createBody); err != nil {
		t.Fatalf("解析 create_worker 响应失败: %v", err)
	}
	if createBody.Result.StructuredContent.Worker.ID == "" {
		t.Fatalf("create_worker 未返回有效 worker")
	}

	mainFile := filepath.Join(dataDir, "workers", createBody.Result.StructuredContent.Worker.ID, "main.py")
	if _, err := os.Stat(mainFile); err != nil {
		t.Fatalf("create_worker 未生成 main.py: %v", err)
	}

	updateResp := doMCPRequest(t, handler, "mcp-token", map[string]any{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "tools/call",
		"params": map[string]any{
			"name": "update_worker",
			"arguments": map[string]any{
				"id":         createBody.Result.StructuredContent.Worker.ID,
				"name":       "订单 Worker V2",
				"route":      "/orders/v2/*",
				"timeout_ms": 4500,
				"enabled":    true,
			},
		},
	})
	if updateResp.Code != http.StatusOK {
		t.Fatalf("update_worker 应返回 200，实际为 %d，body=%s", updateResp.Code, updateResp.Body.String())
	}

	var updateBody struct {
		Result struct {
			IsError           bool `json:"isError"`
			StructuredContent struct {
				Worker model.Worker `json:"worker"`
			} `json:"structuredContent"`
		} `json:"result"`
	}
	if err := json.Unmarshal(updateResp.Body.Bytes(), &updateBody); err != nil {
		t.Fatalf("解析 update_worker 响应失败: %v", err)
	}
	if updateBody.Result.IsError {
		t.Fatalf("更新允许字段不应返回错误: body=%s", updateResp.Body.String())
	}
	if updateBody.Result.StructuredContent.Worker.Name != "订单 Worker V2" {
		t.Fatalf("更新后的 worker name 不正确: %#v", updateBody.Result.StructuredContent.Worker)
	}
	if updateBody.Result.StructuredContent.Worker.Route != "/orders/v2/*" {
		t.Fatalf("更新后的 worker route 不正确: %#v", updateBody.Result.StructuredContent.Worker)
	}
	if updateBody.Result.StructuredContent.Worker.TimeoutMS != 4500 {
		t.Fatalf("更新后的 worker timeout_ms 不正确: %#v", updateBody.Result.StructuredContent.Worker)
	}
}
