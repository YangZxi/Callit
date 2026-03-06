package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"callit/internal/config"
	"callit/internal/db"
	"callit/internal/model"
	"callit/internal/registry"
)

type apiResponse[T any] struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data T      `json:"data"`
}

type logsPageData struct {
	Page     int               `json:"page"`
	PageSize int               `json:"page_size"`
	Total    int               `json:"total"`
	Data     []model.WorkerLog `json:"data"`
}

func TestListWorkerLogsUnauthorized(t *testing.T) {
	engine, _, _ := newAdminTestEngine(t)

	req := httptest.NewRequest(http.MethodGet, "/api/workers/worker-1/logs", nil)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("未授权请求应返回 401，实际 %d", rec.Code)
	}
}

func TestListWorkerLogsInvalidPage(t *testing.T) {
	engine, store, token := newAdminTestEngine(t)
	createTestWorker(t, store, "worker-1")

	req := httptest.NewRequest(http.MethodGet, "/api/workers/worker-1/logs?page=0", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("非法 page 应返回 400，实际 %d", rec.Code)
	}
}

func TestListWorkerLogsWorkerNotFound(t *testing.T) {
	engine, _, token := newAdminTestEngine(t)

	req := httptest.NewRequest(http.MethodGet, "/api/workers/not-exist/logs", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("不存在 worker 应返回 404，实际 %d", rec.Code)
	}
}

func TestListWorkerLogsPagedSuccess(t *testing.T) {
	engine, store, token := newAdminTestEngine(t)
	workerID := "worker-1"
	createTestWorker(t, store, workerID)

	ctx := context.Background()
	for i := 1; i <= 3; i++ {
		if err := store.InsertWorkerLog(ctx, model.WorkerLog{
			ID:         fmt.Sprintf("log-%03d", i),
			WorkerID:   workerID,
			RequestID:  fmt.Sprintf("req-%03d", i),
			Status:     200 + i,
			Stdout:     fmt.Sprintf("stdout-%d", i),
			DurationMS: int64(i),
		}); err != nil {
			t.Fatalf("插入日志失败: %v", err)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/api/workers/worker-1/logs?page=1&page_size=2", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("查询日志应返回 200，实际 %d", rec.Code)
	}

	var resp apiResponse[logsPageData]
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("解析响应失败: %v", err)
	}
	if resp.Code != 200 {
		t.Fatalf("业务码不正确: %d", resp.Code)
	}
	if resp.Data.Page != 1 || resp.Data.PageSize != 2 || resp.Data.Total != 3 {
		t.Fatalf("分页信息不正确: %+v", resp.Data)
	}
	if len(resp.Data.Data) != 2 {
		t.Fatalf("第一页条数不正确: %d", len(resp.Data.Data))
	}
	if resp.Data.Data[0].ID != "log-003" || resp.Data.Data[1].ID != "log-002" {
		t.Fatalf("第一页排序不正确: %#v", resp.Data.Data)
	}

	req2 := httptest.NewRequest(http.MethodGet, "/api/workers/worker-1/logs?page=2&page_size=2", nil)
	req2.Header.Set("Authorization", "Bearer "+token)
	rec2 := httptest.NewRecorder()
	engine.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("第二页应返回 200，实际 %d", rec2.Code)
	}

	var resp2 apiResponse[logsPageData]
	if err := json.Unmarshal(rec2.Body.Bytes(), &resp2); err != nil {
		t.Fatalf("解析第二页响应失败: %v", err)
	}
	if len(resp2.Data.Data) != 1 || resp2.Data.Data[0].ID != "log-001" {
		t.Fatalf("第二页数据不正确: %#v", resp2.Data.Data)
	}
}

func newAdminTestEngine(t *testing.T) (*http.ServeMux, *db.Store, string) {
	t.Helper()
	dataDir := t.TempDir()
	store, err := db.Open(filepath.Join(dataDir, "app.db"))
	if err != nil {
		t.Fatalf("打开数据库失败: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	token := "test-token"
	engine := NewEngine(store, registry.New(), dataDir, token, config.AIConfig{})
	mux := http.NewServeMux()
	mux.Handle("/", engine)
	return mux, store, token
}

func createTestWorker(t *testing.T, store *db.Store, workerID string) {
	t.Helper()
	_, err := store.CreateWorker(context.Background(), model.Worker{
		ID:        workerID,
		Name:      "test-worker",
		Runtime:   "python",
		Route:     "/api/test/*",
		TimeoutMS: 5000,
		Enabled:   true,
	})
	if err != nil {
		t.Fatalf("创建 worker 失败: %v", err)
	}
}
