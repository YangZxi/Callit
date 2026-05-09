package admin

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"callit/internal/admin/message"
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

func openAdminTestStoreAtPath(t *testing.T, dbPath string) *db.Store {
	t.Helper()

	store, err := db.Open(dbPath)
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

func openRawSQLiteDB(t *testing.T, dbPath string) *sql.DB {
	t.Helper()

	rawDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("打开原始 SQLite 连接失败: %v", err)
	}
	t.Cleanup(func() {
		if err := rawDB.Close(); err != nil {
			t.Fatalf("关闭原始 SQLite 连接失败: %v", err)
		}
	})
	return rawDB
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

func doAdminJSONRequestWithoutAuth(t *testing.T, engine http.Handler, method string, path string, body any) *httptest.ResponseRecorder {
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

	resp := httptest.NewRecorder()
	engine.ServeHTTP(resp, req)
	return resp
}

func TestWorkerCronCRUDAPI(t *testing.T) {
	store := openAdminTestStore(t)
	createAdminTestWorker(t, store, "worker-cron-api")

	cfg := config.Config{
		AdminPrefix: "/admin",
		AdminToken:  "test-token",
		DataDir:     t.TempDir(),
	}
	engine := NewEngine(store, router.New(), nil, &cfg)

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

	cfg := config.Config{
		AdminPrefix: "/admin",
		AdminToken:  "test-token",
		DataDir:     t.TempDir(),
	}
	engine := NewEngine(store, router.New(), nil, &cfg)

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

func TestGetWorkerAPIEnvUsesArray(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "app.db")
	store := openAdminTestStoreAtPath(t, dbPath)
	if _, err := store.Worker.Create(context.Background(), model.Worker{
		ID:        "worker-env-array",
		Name:      "测试 Worker",
		Runtime:   "python",
		Route:     "/worker-env-array",
		TimeoutMS: 3000,
		Env:       model.WorkerEnv{"API_KEY=test", "REGION=us"},
		Enabled:   true,
	}); err != nil {
		t.Fatalf("创建测试 Worker 失败: %v", err)
	}

	cfg := config.Config{
		AdminPrefix: "/admin",
		AdminToken:  "test-token",
		DataDir:     t.TempDir(),
	}
	engine := NewEngine(store, router.New(), nil, &cfg)

	resp := doAdminJSONRequest(t, engine, http.MethodGet, "/admin/api/workers/worker-env-array", nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("查询 Worker 接口返回错误: code=%d body=%s", resp.Code, resp.Body.String())
	}

	var body struct {
		Data model.Worker `json:"data"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("解析 Worker 响应失败: %v", err)
	}
	want := model.WorkerEnv{"API_KEY=test", "REGION=us"}
	if len(body.Data.Env) != len(want) {
		t.Fatalf("env 返回数量不正确: %#v", body.Data.Env)
	}
	for i := range want {
		if body.Data.Env[i] != want[i] {
			t.Fatalf("env 返回结果不正确: got=%#v want=%#v", body.Data.Env, want)
		}
	}
}

func TestCreateWorkerAPIAcceptsEnvArray(t *testing.T) {
	store := openAdminTestStore(t)
	cfg := config.Config{
		AdminPrefix: "/admin",
		AdminToken:  "test-token",
		DataDir:     t.TempDir(),
	}
	engine := NewEngine(store, router.New(), nil, &cfg)

	resp := doAdminJSONRequest(t, engine, http.MethodPost, "/admin/api/workers/create", map[string]any{
		"name":       "api-env-worker",
		"runtime":    "python",
		"route":      "/api-env-worker",
		"timeout_ms": 3000,
		"env":        []string{"API_KEY=test", "REGION=us"},
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("创建 Worker 接口返回错误: code=%d body=%s", resp.Code, resp.Body.String())
	}

	var body struct {
		Data model.Worker `json:"data"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("解析创建 Worker 响应失败: %v", err)
	}
	want := model.WorkerEnv{"API_KEY=test", "REGION=us"}
	if len(body.Data.Env) != len(want) {
		t.Fatalf("创建 Worker 返回的 env 数量不正确: %#v", body.Data.Env)
	}
	for i := range want {
		if body.Data.Env[i] != want[i] {
			t.Fatalf("创建 Worker 返回的 env 不正确: got=%#v want=%#v", body.Data.Env, want)
		}
	}
}

func TestListWorkersAPIReadsLegacyStringEnvAsArray(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "app.db")
	store := openAdminTestStoreAtPath(t, dbPath)
	if _, err := store.Worker.Create(context.Background(), model.Worker{
		ID:        "worker-legacy-env",
		Name:      "Legacy Env",
		Runtime:   "python",
		Route:     "/worker-legacy-env",
		TimeoutMS: 3000,
		Env:       model.WorkerEnv{"TEMP=value"},
		Enabled:   true,
	}); err != nil {
		t.Fatalf("创建测试 Worker 失败: %v", err)
	}

	rawDB := openRawSQLiteDB(t, dbPath)
	if _, err := rawDB.Exec("UPDATE worker SET env = ? WHERE id = ?", "API_KEY=test;REGION=us", "worker-legacy-env"); err != nil {
		t.Fatalf("写入旧格式 env 失败: %v", err)
	}

	cfg := config.Config{
		AdminPrefix: "/admin",
		AdminToken:  "test-token",
		DataDir:     t.TempDir(),
	}
	engine := NewEngine(store, router.New(), nil, &cfg)

	resp := doAdminJSONRequest(t, engine, http.MethodGet, "/admin/api/workers?keyword=legacy", nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("查询 workers 接口返回错误: code=%d body=%s", resp.Code, resp.Body.String())
	}

	var body struct {
		Data []model.Worker `json:"data"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("解析 workers 响应失败: %v", err)
	}
	if len(body.Data) != 1 {
		t.Fatalf("workers 返回数量不正确: %#v", body.Data)
	}
	want := model.WorkerEnv{"API_KEY=test", "REGION=us"}
	if len(body.Data[0].Env) != len(want) {
		t.Fatalf("兼容旧格式的 env 数量不正确: %#v", body.Data[0].Env)
	}
	for i := range want {
		if body.Data[0].Env[i] != want[i] {
			t.Fatalf("兼容旧格式的 env 不正确: got=%#v want=%#v", body.Data[0].Env, want)
		}
	}
}

func TestDashboardMetricsAPIRequiresAuthAndReturnsData(t *testing.T) {
	store := openAdminTestStore(t)
	createAdminTestWorker(t, store, "worker-dashboard-metrics")

	cfg := config.Config{
		AdminPrefix: "/admin",
		AdminToken:  "test-token",
		DataDir:     t.TempDir(),
	}
	engine := NewEngine(store, router.New(), nil, &cfg)

	unauthResp := doAdminJSONRequestWithoutAuth(t, engine, http.MethodGet, "/admin/api/dashboard/metrics", nil)
	if unauthResp.Code != http.StatusUnauthorized {
		t.Fatalf("未认证访问 dashboard metrics 应返回 401: code=%d body=%s", unauthResp.Code, unauthResp.Body.String())
	}

	resp := doAdminJSONRequest(t, engine, http.MethodGet, "/admin/api/dashboard/metrics", nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("dashboard metrics 接口应返回 200: code=%d body=%s", resp.Code, resp.Body.String())
	}

	var body struct {
		Data message.DashboardMetricsResponse `json:"data"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("解析 dashboard metrics 响应失败: %v", err)
	}
	if body.Data.Workers.Total != 1 || body.Data.Workers.Enabled != 1 {
		t.Fatalf("dashboard metrics Worker 数量不正确: %#v", body.Data.Workers)
	}
}

func TestDashboardWorkerTrendAPI(t *testing.T) {
	store := openAdminTestStore(t)
	createAdminTestWorker(t, store, "worker-dashboard-trend")

	cfg := config.Config{
		AdminPrefix: "/admin",
		AdminToken:  "test-token",
		DataDir:     t.TempDir(),
	}
	engine := NewEngine(store, router.New(), nil, &cfg)

	resp := doAdminJSONRequest(t, engine, http.MethodGet, "/admin/api/dashboard/workers/trend?worker_id=all", nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("dashboard trend 接口应返回 200: code=%d body=%s", resp.Code, resp.Body.String())
	}

	var body struct {
		Data []message.DashboardWorkerTrendPoint `json:"data"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("解析 dashboard trend 响应失败: %v", err)
	}
	if len(body.Data) != 24 {
		t.Fatalf("dashboard trend 应返回 24 个点，实际 %d", len(body.Data))
	}

	notFoundResp := doAdminJSONRequest(t, engine, http.MethodGet, "/admin/api/dashboard/workers/trend?worker_id=not-exist", nil)
	if notFoundResp.Code != http.StatusNotFound {
		t.Fatalf("不存在 Worker 的 trend 查询应返回 404: code=%d body=%s", notFoundResp.Code, notFoundResp.Body.String())
	}
}

func TestDashboardAPIIgnoresDeletedWorkerLogs(t *testing.T) {
	store := openAdminTestStore(t)
	createAdminTestWorker(t, store, "worker-dashboard-active")
	createAdminTestWorker(t, store, "worker-dashboard-deleted")

	now := time.Now().UTC()
	if err := store.WorkerLog.Insert(context.Background(), model.WorkerLog{
		WorkerID:   "worker-dashboard-active",
		RequestID:  "active-success",
		Status:     http.StatusOK,
		DurationMS: 100,
		CreatedAt:  now.Add(-10 * time.Minute),
	}); err != nil {
		t.Fatalf("写入当前 Worker 日志失败: %v", err)
	}
	if err := store.WorkerLog.Insert(context.Background(), model.WorkerLog{
		WorkerID:   "worker-dashboard-deleted",
		RequestID:  "deleted-failed",
		Status:     http.StatusInternalServerError,
		DurationMS: 900,
		CreatedAt:  now.Add(-5 * time.Minute),
	}); err != nil {
		t.Fatalf("写入已删除 Worker 日志失败: %v", err)
	}
	if err := store.Worker.Delete(context.Background(), "worker-dashboard-deleted"); err != nil {
		t.Fatalf("删除测试 Worker 失败: %v", err)
	}

	cfg := config.Config{
		AdminPrefix: "/admin",
		AdminToken:  "test-token",
		DataDir:     t.TempDir(),
	}
	engine := NewEngine(store, router.New(), nil, &cfg)

	metricsResp := doAdminJSONRequest(t, engine, http.MethodGet, "/admin/api/dashboard/metrics", nil)
	if metricsResp.Code != http.StatusOK {
		t.Fatalf("dashboard metrics 接口应返回 200: code=%d body=%s", metricsResp.Code, metricsResp.Body.String())
	}
	var metricsBody struct {
		Data message.DashboardMetricsResponse `json:"data"`
	}
	if err := json.Unmarshal(metricsResp.Body.Bytes(), &metricsBody); err != nil {
		t.Fatalf("解析 dashboard metrics 响应失败: %v", err)
	}
	if metricsBody.Data.Summary.TotalCalls24h != 1 || metricsBody.Data.Summary.FailedCalls24h != 0 {
		t.Fatalf("metrics 不应统计已删除 Worker 日志: %#v", metricsBody.Data.Summary)
	}

	trendResp := doAdminJSONRequest(t, engine, http.MethodGet, "/admin/api/dashboard/workers/trend?worker_id=all", nil)
	if trendResp.Code != http.StatusOK {
		t.Fatalf("dashboard trend 接口应返回 200: code=%d body=%s", trendResp.Code, trendResp.Body.String())
	}
	var trendBody struct {
		Data []message.DashboardWorkerTrendPoint `json:"data"`
	}
	if err := json.Unmarshal(trendResp.Body.Bytes(), &trendBody); err != nil {
		t.Fatalf("解析 dashboard trend 响应失败: %v", err)
	}
	total := 0
	failed := 0
	for _, point := range trendBody.Data {
		total += point.Total
		failed += point.Failed
	}
	if total != 1 || failed != 0 {
		t.Fatalf("trend 不应统计已删除 Worker 日志: total=%d failed=%d data=%#v", total, failed, trendBody.Data)
	}
}

func TestWorkerCronCreateRejectsInvalidCron(t *testing.T) {
	store := openAdminTestStore(t)
	createAdminTestWorker(t, store, "worker-cron-invalid")

	cfg := config.Config{
		AdminPrefix: "/admin",
		AdminToken:  "test-token",
		DataDir:     t.TempDir(),
	}
	engine := NewEngine(store, router.New(), nil, &cfg)

	resp := doAdminJSONRequest(t, engine, http.MethodPost, "/admin/api/workers/worker-cron-invalid/crons/create", map[string]any{
		"cron": "invalid cron",
	})
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("非法 cron 表达式应返回 400: code=%d body=%s", resp.Code, resp.Body.String())
	}
}

func TestAdminConfigAPIIncludesAndUpdatesMCPConfig(t *testing.T) {
	store := openAdminTestStore(t)
	cfg := config.Config{
		AdminPrefix: "/admin",
		AdminToken:  "test-token",
		DataDir:     t.TempDir(),
		AppConfig: config.AppConfig{
			MCP_Enable: false,
			MCP_Token:  "",
		},
	}
	engine := NewEngine(store, router.New(), nil, &cfg)

	getResp := doAdminJSONRequest(t, engine, http.MethodGet, "/admin/api/config", nil)
	if getResp.Code != http.StatusOK {
		t.Fatalf("读取配置接口返回错误: code=%d body=%s", getResp.Code, getResp.Body.String())
	}

	var getBody struct {
		Data []message.AdminConfigItem `json:"data"`
	}
	if err := json.Unmarshal(getResp.Body.Bytes(), &getBody); err != nil {
		t.Fatalf("解析配置读取响应失败: %v", err)
	}

	foundEnable := false
	foundToken := false
	for _, item := range getBody.Data {
		if item.Key == "MCP_ENABLE" {
			foundEnable = true
		}
		if item.Key == "MCP_TOKEN" {
			foundToken = true
		}
	}
	if !foundEnable || !foundToken {
		t.Fatalf("配置列表未包含 MCP 配置项: %#v", getBody.Data)
	}

	updateResp := doAdminJSONRequest(t, engine, http.MethodPost, "/admin/api/config", map[string]any{
		"app_config": map[string]any{
			"MCP_ENABLE": "true",
			"MCP_TOKEN":  "new-mcp-token",
		},
	})
	if updateResp.Code != http.StatusOK {
		t.Fatalf("更新配置接口返回错误: code=%d body=%s", updateResp.Code, updateResp.Body.String())
	}

	if cfg.AppConfig.MCP_Enable != true {
		t.Fatalf("期望 MCP_ENABLE 已更新为 true")
	}
	if cfg.AppConfig.MCP_Token != "new-mcp-token" {
		t.Fatalf("期望 MCP_TOKEN 已更新，实际为 %q", cfg.AppConfig.MCP_Token)
	}
}

func TestUpdateWorkerWithoutRuntimeField(t *testing.T) {
	store := openAdminTestStore(t)
	createAdminTestWorker(t, store, "worker-update-http")

	cfg := config.Config{
		AdminPrefix: "/admin",
		AdminToken:  "test-token",
		DataDir:     t.TempDir(),
	}
	engine := NewEngine(store, router.New(), nil, &cfg)

	resp := doAdminJSONRequest(t, engine, http.MethodPost, "/admin/api/workers/update", map[string]any{
		"id":         "worker-update-http",
		"name":       "更新后的 Worker",
		"route":      "/worker-update-http-v2",
		"timeout_ms": 4500,
		"enabled":    true,
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("不携带 runtime 的更新接口应返回 200: code=%d body=%s", resp.Code, resp.Body.String())
	}

	var body struct {
		Data model.Worker `json:"data"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("解析更新接口响应失败: %v", err)
	}
	if body.Data.Runtime != "python" {
		t.Fatalf("更新时不应修改 runtime: %#v", body.Data)
	}
	if body.Data.Route != "/worker-update-http-v2" {
		t.Fatalf("更新后的 route 不正确: %#v", body.Data)
	}
}

func TestEnableWorkerSyncsMetadata(t *testing.T) {
	store := openAdminTestStore(t)
	created, err := store.Worker.Create(context.Background(), model.Worker{
		ID:        "worker-enable-http",
		Name:      "启停测试 Worker",
		Runtime:   "python",
		Route:     "/worker-enable-http",
		TimeoutMS: 3000,
		Enabled:   false,
	})
	if err != nil {
		t.Fatalf("创建测试 Worker 失败: %v", err)
	}

	dataDir := t.TempDir()
	workerDir := filepath.Join(dataDir, "workers", created.ID)
	if err := os.MkdirAll(filepath.Join(workerDir, "code"), 0o755); err != nil {
		t.Fatalf("创建 code 目录失败: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workerDir, "metadata.json"), []byte(`{"enabled":false}`), 0o644); err != nil {
		t.Fatalf("写入初始 metadata 失败: %v", err)
	}

	cfg := config.Config{
		AdminPrefix: "/admin",
		AdminToken:  "test-token",
		DataDir:     dataDir,
	}
	engine := NewEngine(store, router.New(), nil, &cfg)

	resp := doAdminJSONRequest(t, engine, http.MethodPost, "/admin/api/workers/enable", map[string]any{
		"id": created.ID,
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("启用接口应返回 200: code=%d body=%s", resp.Code, resp.Body.String())
	}

	raw, err := os.ReadFile(filepath.Join(workerDir, "metadata.json"))
	if err != nil {
		t.Fatalf("读取 metadata 失败: %v", err)
	}
	var metadata model.Worker
	if err := json.Unmarshal(raw, &metadata); err != nil {
		t.Fatalf("解析 metadata 失败: %v", err)
	}
	if !metadata.Enabled {
		t.Fatalf("metadata enabled 未同步: %#v", metadata)
	}
}

func TestDeleteWorkerSoftDeletesWorkerDir(t *testing.T) {
	store := openAdminTestStore(t)
	createAdminTestWorker(t, store, "worker-delete-soft")

	dataDir := t.TempDir()
	workerDir := filepath.Join(dataDir, "workers", "worker-delete-soft")
	if err := os.MkdirAll(filepath.Join(workerDir, "code"), 0o755); err != nil {
		t.Fatalf("创建 code 目录失败: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(workerDir, "data"), 0o755); err != nil {
		t.Fatalf("创建 data 目录失败: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workerDir, "data", "users.db"), []byte("demo"), 0o644); err != nil {
		t.Fatalf("写入 data 文件失败: %v", err)
	}

	cfg := config.Config{
		AdminPrefix: "/admin",
		AdminToken:  "test-token",
		DataDir:     dataDir,
	}
	engine := NewEngine(store, router.New(), nil, &cfg)

	resp := doAdminJSONRequest(t, engine, http.MethodPost, "/admin/api/workers/delete", map[string]any{
		"id": "worker-delete-soft",
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("删除接口应返回 200: code=%d body=%s", resp.Code, resp.Body.String())
	}

	if _, err := store.Worker.GetByID(context.Background(), "worker-delete-soft"); err == nil {
		t.Fatalf("Worker 数据库记录应已删除")
	}
	if _, err := os.Stat(workerDir); !os.IsNotExist(err) {
		t.Fatalf("原目录应不存在，err=%v", err)
	}

	deletedDir := filepath.Join(dataDir, "workers", "deleted_worker-delete-soft")
	if _, err := os.Stat(filepath.Join(deletedDir, "data", "users.db")); err != nil {
		t.Fatalf("软删除目录应保留 data 文件: %v", err)
	}
}
