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
	svc := NewWorkerService(store, nil, nil, workersDir, filepath.Join(t.TempDir(), "tmp"), filepath.Join(t.TempDir(), ".lib"), 3)

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
	svc := NewWorkerService(store, nil, nil, workersDir, filepath.Join(t.TempDir(), "tmp"), filepath.Join(t.TempDir(), ".lib"), 3)

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

func TestWorkerServiceSupportsNestedFilesAndDirectories(t *testing.T) {
	store := openAdminTestStore(t)
	baseDir := t.TempDir()
	workersDir := filepath.Join(baseDir, "workers")
	svc := NewWorkerService(store, nil, nil, workersDir, filepath.Join(baseDir, "tmp"), filepath.Join(baseDir, ".lib"), 3)
	created, err := svc.CreateWorker(context.Background(), CreateWorkerInput{
		Name:      "nested-files",
		Runtime:   "python",
		Route:     "/nested-files",
		TimeoutMS: 3000,
	})
	if err != nil {
		t.Fatalf("CreateWorker 失败: %v", err)
	}

	if _, err := svc.SaveWorkerFileContent(context.Background(), created.ID, "empty/", "ignored"); err != nil {
		t.Fatalf("创建空目录失败: %v", err)
	}
	entries, err := svc.SaveWorkerFileContent(context.Background(), created.ID, " / src / lib / helper.py ", "VALUE = 1")
	if err != nil {
		t.Fatalf("创建嵌套文件失败: %v", err)
	}
	wantEntries := []string{"empty/", "main.py", "src/", "src/lib/", "src/lib/helper.py"}
	if len(entries) != len(wantEntries) {
		t.Fatalf("文件列表不正确: got=%#v want=%#v", entries, wantEntries)
	}
	for i := range wantEntries {
		if entries[i] != wantEntries[i] {
			t.Fatalf("文件列表不正确: got=%#v want=%#v", entries, wantEntries)
		}
	}

	content, err := svc.GetWorkerFile(context.Background(), created.ID, "src/lib/helper.py")
	if err != nil {
		t.Fatalf("读取嵌套文件失败: %v", err)
	}
	if content.Content != "VALUE = 1" || content.Filename != "src/lib/helper.py" {
		t.Fatalf("嵌套文件内容不正确: %#v", content)
	}
	if _, err := svc.GetWorkerFile(context.Background(), created.ID, "src/"); !errors.Is(err, ErrInvalidFilename) {
		t.Fatalf("读取目录应返回 ErrInvalidFilename，got=%v", err)
	}
	if _, err := svc.SaveWorkerFileContent(context.Background(), created.ID, "src/lib/deep/helper.py", ""); !errors.Is(err, ErrInvalidFilename) {
		t.Fatalf("超过最大层级应返回 ErrInvalidFilename，got=%v", err)
	}

	_, renamed, err := svc.RenameWorkerFile(context.Background(), created.ID, "src/lib/helper.py", "src/renamed.py")
	if err != nil {
		t.Fatalf("重命名嵌套文件失败: %v", err)
	}
	if renamed != "src/renamed.py" {
		t.Fatalf("重命名结果不正确: %q", renamed)
	}
	if err := svc.DeleteWorkerFile(context.Background(), created.ID, "src/"); err != nil {
		t.Fatalf("递归删除目录失败: %v", err)
	}
	if _, err := os.Stat(filepath.Join(workersDir, created.ID, "code", "src")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("目录及子文件应已删除，got=%v", err)
	}
	if err := svc.DeleteWorkerFile(context.Background(), created.ID, "src/"); !errors.Is(err, ErrFileNotFound) {
		t.Fatalf("重复删除目录应返回 ErrFileNotFound，got=%v", err)
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

func TestNormalizeWorkerEnvUsesSingleNewlineAndKeepsSemicolonInValue(t *testing.T) {
	normalized := normalizeWorkerEnv(" API_KEY=test \r\n\r\n QQ_MUSIC_COOKIE=uin=o1282381264;wxuin=;euin=abc \n\n REGION=us \r\n")
	want := "API_KEY=test\nQQ_MUSIC_COOKIE=uin=o1282381264;wxuin=;euin=abc\nREGION=us"
	if normalized != want {
		t.Fatalf("环境变量规范化结果不正确，got=%q want=%q", normalized, want)
	}
}
