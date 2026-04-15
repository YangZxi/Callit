package admin

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
