package admin

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"callit/internal/model"
)

func TestCreateWorkerCreatesNodeESMTemplateAndPackageJSON(t *testing.T) {
	store := openAdminTestStore(t)
	workersDir := filepath.Join(t.TempDir(), "workers")
	svc := NewWorkerService(store, nil, nil, workersDir, filepath.Join(t.TempDir(), "tmp"), filepath.Join(t.TempDir(), ".lib"))

	created, err := svc.CreateWorker(context.Background(), CreateWorkerInput{
		Name:      "node-esm",
		Runtime:   "node",
		Route:     "/node-esm",
		TimeoutMS: 3000,
	})
	if err != nil {
		t.Fatalf("CreateWorker 失败: %v", err)
	}

	mainContent, err := os.ReadFile(filepath.Join(workersDir, created.ID, "code", "main.js"))
	if err != nil {
		t.Fatalf("读取 main.js 失败: %v", err)
	}
	if !strings.Contains(string(mainContent), "export default") {
		t.Fatalf("新建 Node Worker 应默认生成 ESM 模板，content=%q", string(mainContent))
	}

	packageContent, err := os.ReadFile(filepath.Join(workersDir, created.ID, "code", "package.json"))
	if err != nil {
		t.Fatalf("读取 package.json 失败: %v", err)
	}
	if !strings.Contains(string(packageContent), `"type": "module"`) {
		t.Fatalf("新建 Node Worker 应默认生成 type=module，content=%q", string(packageContent))
	}
}

func TestDeleteWorkerFileRejectsMainJS(t *testing.T) {
	store := openAdminTestStore(t)
	workersDir := filepath.Join(t.TempDir(), "workers")
	svc := NewWorkerService(store, nil, nil, workersDir, filepath.Join(t.TempDir(), "tmp"), filepath.Join(t.TempDir(), ".lib"))

	created, err := svc.CreateWorker(context.Background(), CreateWorkerInput{
		Name:      "node-main-protect",
		Runtime:   "node",
		Route:     "/node-main-protect",
		TimeoutMS: 3000,
	})
	if err != nil {
		t.Fatalf("CreateWorker 失败: %v", err)
	}

	err = svc.DeleteWorkerFile(context.Background(), created.ID, "main.js")
	if !errors.Is(err, ErrMainFileDeletion) {
		t.Fatalf("删除 main.js 应返回 ErrMainFileDeletion，got=%v", err)
	}
}

func TestCreateNodeWorkerPackageJSONTemplate(t *testing.T) {
	workerDir := t.TempDir()

	if err := createNodeWorkerPackageJSON(workerDir); err != nil {
		t.Fatalf("创建 Node Worker package.json 失败: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(workerDir, "package.json"))
	if err != nil {
		t.Fatalf("读取 package.json 失败: %v", err)
	}
	if string(content) != "{\n  \"type\": \"module\"\n}\n" {
		t.Fatalf("package.json 模板不正确，got=%q", string(content))
	}
}

func TestMainFilenameByRuntimeForNode(t *testing.T) {
	if got := mainFilenameByRuntime("node"); got != "main.js" {
		t.Fatalf("node runtime 主文件名不正确，got=%q", got)
	}
}

func TestSanitizeWorkerEnvKeepsSemicolonInValue(t *testing.T) {
	normalized := sanitizeWorkerEnv([]string{" API_KEY=test ", "", " QQ_MUSIC_COOKIE=uin=o1282381264;wxuin=;euin=abc ", " REGION=us "})
	want := model.WorkerEnv{"API_KEY=test", "QQ_MUSIC_COOKIE=uin=o1282381264;wxuin=;euin=abc", "REGION=us"}
	if len(normalized) != len(want) {
		t.Fatalf("环境变量规范化结果数量不正确，got=%#v want=%#v", normalized, want)
	}
	for i := range want {
		if normalized[i] != want[i] {
			t.Fatalf("环境变量规范化结果不正确，got=%#v want=%#v", normalized, want)
		}
	}
}

func TestCreateWorkerStoresEnvAsJSONArrayStringAndMetadata(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "app.db")
	store := openAdminTestStoreAtPath(t, dbPath)
	workersDir := filepath.Join(t.TempDir(), "workers")
	svc := NewWorkerService(store, nil, nil, workersDir, filepath.Join(t.TempDir(), "tmp"), filepath.Join(t.TempDir(), ".lib"))

	created, err := svc.CreateWorker(context.Background(), CreateWorkerInput{
		Name:      "env-worker",
		Runtime:   "python",
		Route:     "/env-worker",
		TimeoutMS: 3000,
		Env:       []string{" API_KEY=test ", "REGION=us"},
	})
	if err != nil {
		t.Fatalf("CreateWorker 失败: %v", err)
	}

	rawDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("打开原始 SQLite 连接失败: %v", err)
	}
	defer func() {
		if err := rawDB.Close(); err != nil {
			t.Fatalf("关闭原始 SQLite 连接失败: %v", err)
		}
	}()

	var storedEnv string
	if err := rawDB.QueryRow("SELECT env FROM worker WHERE id = ?", created.ID).Scan(&storedEnv); err != nil {
		t.Fatalf("查询落库 env 失败: %v", err)
	}
	if storedEnv != "[\"API_KEY=test\",\"REGION=us\"]" {
		t.Fatalf("env 落库存储格式不正确，got=%q", storedEnv)
	}

	raw, err := os.ReadFile(filepath.Join(workersDir, created.ID, "metadata.json"))
	if err != nil {
		t.Fatalf("读取 metadata.json 失败: %v", err)
	}
	var meta model.Worker
	if err := json.Unmarshal(raw, &meta); err != nil {
		t.Fatalf("解析 metadata.json 失败: %v", err)
	}
	want := model.WorkerEnv{"API_KEY=test", "REGION=us"}
	if len(meta.Env) != len(want) {
		t.Fatalf("metadata env 数量不正确: %#v", meta.Env)
	}
	for i := range want {
		if meta.Env[i] != want[i] {
			t.Fatalf("metadata env 不正确: got=%#v want=%#v", meta.Env, want)
		}
	}
}
