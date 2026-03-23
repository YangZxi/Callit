package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"

	"callit/internal/config"
	"callit/internal/db"
	"callit/internal/model"
	"callit/internal/router"
)

func TestIsAdminAPIPath(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{path: "/admin/api/auth/status", want: true},
		{path: "/admin/api/workers", want: true},
		{path: "/admin", want: false},
		{path: "/api/auth/status", want: false},
	}

	for _, tc := range tests {
		got := isAdminAPIPath(tc.path, "/admin")
		if got != tc.want {
			t.Fatalf("unexpected admin api path result for %q: got=%v want=%v", tc.path, got, tc.want)
		}
	}
}

func openAdminTestStore(t *testing.T) *db.Store {
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

func createAdminTestWorker(t *testing.T, store *db.Store, workerID string) {
	t.Helper()

	if _, err := store.Worker.Create(context.Background(), model.Worker{
		ID:        workerID,
		Name:      "测试函数",
		Runtime:   "python",
		Route:     "/" + workerID,
		TimeoutMS: 3000,
		Enabled:   true,
	}); err != nil {
		t.Fatalf("创建测试 Worker 失败: %v", err)
	}
}

func createAdminTestWorkerWithName(t *testing.T, store *db.Store, workerID string, workerName string) {
	t.Helper()

	if _, err := store.Worker.Create(context.Background(), model.Worker{
		ID:        workerID,
		Name:      workerName,
		Runtime:   "python",
		Route:     "/" + workerID,
		TimeoutMS: 3000,
		Enabled:   true,
	}); err != nil {
		t.Fatalf("创建测试 Worker 失败: %v", err)
	}
}

func doAdminJSONRequest(t *testing.T, engine http.Handler, method string, path string, body any) *httptest.ResponseRecorder {
	t.Helper()

	var payload []byte
	var err error
	if body != nil {
		payload, err = json.Marshal(body)
		if err != nil {
			t.Fatalf("序列化请求体失败: %v", err)
		}
	}

	req := httptest.NewRequest(method, path, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-token")

	resp := httptest.NewRecorder()
	engine.ServeHTTP(resp, req)
	return resp
}

func TestWorkerCronCRUDAPI(t *testing.T) {
	store := openAdminTestStore(t)
	createAdminTestWorker(t, store, "worker-cron-api")

	engine := NewEngine(store, router.New(), nil, config.Config{
		AdminPrefix: "/admin",
		AdminToken:  "test-token",
		DataDir:     t.TempDir(),
	})

	createResp := doAdminJSONRequest(t, engine, http.MethodPost, "/admin/api/workers/worker-cron-api/crons/create", map[string]any{
		"cron": "*/5 * * * *",
	})
	if createResp.Code != http.StatusOK {
		t.Fatalf("创建 cron_task 接口返回错误: code=%d body=%s", createResp.Code, createResp.Body.String())
	}

	var createBody struct {
		Data model.CronTask `json:"data"`
	}
	if err := json.Unmarshal(createResp.Body.Bytes(), &createBody); err != nil {
		t.Fatalf("解析创建接口响应失败: %v", err)
	}
	if createBody.Data.ID <= 0 {
		t.Fatalf("创建接口返回的 cron_task id 不正确: %#v", createBody.Data)
	}

	listResp := doAdminJSONRequest(t, engine, http.MethodGet, "/admin/api/workers/worker-cron-api/crons", nil)
	if listResp.Code != http.StatusOK {
		t.Fatalf("查询 cron_task 接口返回错误: code=%d body=%s", listResp.Code, listResp.Body.String())
	}
	var listBody struct {
		Data []model.CronTask `json:"data"`
	}
	if err := json.Unmarshal(listResp.Body.Bytes(), &listBody); err != nil {
		t.Fatalf("解析查询接口响应失败: %v", err)
	}
	if len(listBody.Data) != 1 || listBody.Data[0].Cron != "*/5 * * * *" {
		t.Fatalf("查询接口返回结果不正确: %#v", listBody.Data)
	}

	updateResp := doAdminJSONRequest(t, engine, http.MethodPost, "/admin/api/workers/worker-cron-api/crons/update", map[string]any{
		"id":   strconv.FormatInt(createBody.Data.ID, 10),
		"cron": "0 * * * *",
	})
	if updateResp.Code != http.StatusOK {
		t.Fatalf("更新 cron_task 接口返回错误: code=%d body=%s", updateResp.Code, updateResp.Body.String())
	}

	deleteResp := doAdminJSONRequest(t, engine, http.MethodPost, "/admin/api/workers/worker-cron-api/crons/delete", map[string]any{
		"id": strconv.FormatInt(createBody.Data.ID, 10),
	})
	if deleteResp.Code != http.StatusOK {
		t.Fatalf("删除 cron_task 接口返回错误: code=%d body=%s", deleteResp.Code, deleteResp.Body.String())
	}

	finalListResp := doAdminJSONRequest(t, engine, http.MethodGet, "/admin/api/workers/worker-cron-api/crons", nil)
	if finalListResp.Code != http.StatusOK {
		t.Fatalf("删除后查询 cron_task 接口返回错误: code=%d body=%s", finalListResp.Code, finalListResp.Body.String())
	}
	if err := json.Unmarshal(finalListResp.Body.Bytes(), &listBody); err != nil {
		t.Fatalf("解析删除后查询接口响应失败: %v", err)
	}
	if len(listBody.Data) != 0 {
		t.Fatalf("删除后仍存在 cron_task: %#v", listBody.Data)
	}
}

func TestListWorkersSupportsKeywordFilter(t *testing.T) {
	store := openAdminTestStore(t)
	createAdminTestWorkerWithName(t, store, "worker-alpha", "Alpha Worker")
	createAdminTestWorkerWithName(t, store, "worker-beta", "Beta Worker")
	createAdminTestWorkerWithName(t, store, "worker-api", "支付API")

	engine := NewEngine(store, router.New(), nil, config.Config{
		AdminPrefix: "/admin",
		AdminToken:  "test-token",
		DataDir:     t.TempDir(),
	})

	resp := doAdminJSONRequest(t, engine, http.MethodGet, "/admin/api/workers?keyword=api", nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("按 keyword 查询 workers 接口返回错误: code=%d body=%s", resp.Code, resp.Body.String())
	}

	var body struct {
		Data []model.Worker `json:"data"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("解析 workers 查询响应失败: %v", err)
	}
	if len(body.Data) != 1 {
		t.Fatalf("keyword 查询结果数量不正确: got=%d data=%#v", len(body.Data), body.Data)
	}
	if body.Data[0].ID != "worker-api" {
		t.Fatalf("keyword 查询命中错误 worker: %#v", body.Data[0])
	}

	fullResp := doAdminJSONRequest(t, engine, http.MethodGet, "/admin/api/workers?keyword=", nil)
	if fullResp.Code != http.StatusOK {
		t.Fatalf("空 keyword 查询 workers 接口返回错误: code=%d body=%s", fullResp.Code, fullResp.Body.String())
	}
	if err := json.Unmarshal(fullResp.Body.Bytes(), &body); err != nil {
		t.Fatalf("解析空 keyword 查询响应失败: %v", err)
	}
	if len(body.Data) != 3 {
		t.Fatalf("空 keyword 应返回全部 worker: got=%d data=%#v", len(body.Data), body.Data)
	}
}

func TestWorkerCronCreateRejectsInvalidCron(t *testing.T) {
	store := openAdminTestStore(t)
	createAdminTestWorker(t, store, "worker-cron-invalid")

	engine := NewEngine(store, router.New(), nil, config.Config{
		AdminPrefix: "/admin",
		AdminToken:  "test-token",
		DataDir:     t.TempDir(),
	})

	resp := doAdminJSONRequest(t, engine, http.MethodPost, "/admin/api/workers/worker-cron-invalid/crons/create", map[string]any{
		"cron": "invalid cron",
	})
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("非法 cron 表达式应返回 400: code=%d body=%s", resp.Code, resp.Body.String())
	}
}
