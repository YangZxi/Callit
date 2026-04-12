package admin

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"callit/internal/db"
	"callit/internal/model"
	"callit/internal/router"

	"github.com/google/uuid"
)

var (
	ErrWorkerNotFound       = errors.New("worker not found")
	ErrWorkerRouteExists    = errors.New("worker route exists")
	ErrFileNotFound         = errors.New("file not found")
	ErrInvalidFilename      = errors.New("invalid filename")
	ErrMainFileDeletion     = errors.New("main file cannot be deleted")
	ErrBinaryFileNotSupport = errors.New("binary file not supported")
)

type WorkerService struct {
	store        *db.Store
	reg          *router.Registry
	cronReloader interface{ Reload(context.Context) error }
	dataDir      string
}

type CreateWorkerInput struct {
	Name        string
	Description string
	Runtime     string
	Route       string
	TimeoutMS   int
	Env         string
	Enabled     *bool
}

type UpdateWorkerInput struct {
	ID          string
	Name        string
	Description string
	Route       string
	TimeoutMS   int
	Env         string
	Enabled     *bool
}

type FileContent struct {
	Filename       string `json:"filename"`
	Content        string `json:"content"`
	MediaType      string `json:"media_type"`
	MIMEType       string `json:"mime_type"`
	PreviewDataURL string `json:"preview_data_url,omitempty"`
}

func NewWorkerService(store *db.Store, reg *router.Registry, cronReloader interface{ Reload(context.Context) error }, dataDir string) *WorkerService {
	return &WorkerService{
		store:        store,
		reg:          reg,
		cronReloader: cronReloader,
		dataDir:      dataDir,
	}
}

func (s *WorkerService) SearchWorkers(ctx context.Context, name string) ([]model.Worker, error) {
	return s.store.Worker.List(ctx, strings.TrimSpace(name))
}

func (s *WorkerService) GetWorker(ctx context.Context, id string) (model.Worker, error) {
	worker, err := s.store.Worker.GetByID(ctx, strings.TrimSpace(id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.Worker{}, ErrWorkerNotFound
		}
		return model.Worker{}, err
	}
	return worker, nil
}

func (s *WorkerService) CreateWorker(ctx context.Context, input CreateWorkerInput) (model.Worker, error) {
	enabled := true
	if input.Enabled != nil {
		enabled = *input.Enabled
	}
	timeoutMS := input.TimeoutMS
	if timeoutMS <= 0 {
		timeoutMS = 5000
	}

	worker := model.Worker{
		ID:          uuid.NewString(),
		Name:        strings.TrimSpace(input.Name),
		Description: strings.TrimSpace(input.Description),
		Runtime:     strings.TrimSpace(input.Runtime),
		Route:       strings.TrimSpace(input.Route),
		TimeoutMS:   timeoutMS,
		Env:         normalizeWorkerEnv(input.Env),
		Enabled:     enabled,
	}
	if err := worker.Validate(); err != nil {
		return model.Worker{}, err
	}

	created, err := s.store.Worker.Create(ctx, worker)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed: worker.route") {
			return model.Worker{}, ErrWorkerRouteExists
		}
		return model.Worker{}, err
	}

	workerDir := filepath.Join(s.dataDir, "workers", created.ID)
	if err := os.MkdirAll(workerDir, 0o755); err != nil {
		_ = s.store.Worker.Delete(context.Background(), created.ID)
		return model.Worker{}, fmt.Errorf("创建函数目录失败: %w", err)
	}
	if err := createMainFileFromTemplate(workerDir, created.Runtime); err != nil {
		_ = os.RemoveAll(workerDir)
		_ = s.store.Worker.Delete(context.Background(), created.ID)
		return model.Worker{}, fmt.Errorf("创建入口文件失败: %w", err)
	}
	if err := s.reloadWorkersState(ctx); err != nil {
		return model.Worker{}, err
	}
	return created, nil
}

func (s *WorkerService) UpdateWorker(ctx context.Context, input UpdateWorkerInput) (model.Worker, error) {
	id := strings.TrimSpace(input.ID)
	if id == "" {
		return model.Worker{}, fmt.Errorf("id 不能为空")
	}

	origin, err := s.GetWorker(ctx, id)
	if err != nil {
		return model.Worker{}, err
	}

	timeoutMS := input.TimeoutMS
	if timeoutMS <= 0 {
		timeoutMS = 5000
	}

	enabled := origin.Enabled
	if input.Enabled != nil {
		enabled = *input.Enabled
	}

	updating := model.Worker{
		ID:          id,
		Name:        strings.TrimSpace(input.Name),
		Description: strings.TrimSpace(input.Description),
		Runtime:     origin.Runtime,
		Route:       strings.TrimSpace(input.Route),
		TimeoutMS:   timeoutMS,
		Env:         normalizeWorkerEnv(input.Env),
		Enabled:     enabled,
	}
	if err := updating.Validate(); err != nil {
		return model.Worker{}, err
	}

	updated, err := s.store.Worker.Update(ctx, updating)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.Worker{}, ErrWorkerNotFound
		}
		if strings.Contains(err.Error(), "UNIQUE constraint failed: worker.route") {
			return model.Worker{}, ErrWorkerRouteExists
		}
		return model.Worker{}, err
	}
	if err := s.reloadWorkersState(ctx); err != nil {
		return model.Worker{}, err
	}
	return updated, nil
}

func normalizeWorkerEnv(envText string) string {
	items := strings.FieldsFunc(envText, func(r rune) bool {
		return r == ';' || r == '\n'
	})
	normalized := make([]string, 0, len(items))
	for _, item := range items {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		normalized = append(normalized, trimmed)
	}
	return strings.Join(normalized, ";")
}

func (s *WorkerService) ListWorkerFiles(ctx context.Context, id string) ([]string, error) {
	if _, err := s.GetWorker(ctx, id); err != nil {
		return nil, err
	}

	workerDir := filepath.Join(s.dataDir, "workers", id)
	if err := os.MkdirAll(workerDir, 0o755); err != nil {
		return nil, fmt.Errorf("创建函数目录失败: %w", err)
	}
	return listFiles(workerDir)
}

func (s *WorkerService) GetWorkerFile(ctx context.Context, id string, filename string) (FileContent, error) {
	filename = strings.TrimSpace(filename)
	if err := validateFilename(filename); err != nil {
		return FileContent{}, err
	}
	if _, err := s.GetWorker(ctx, id); err != nil {
		return FileContent{}, err
	}

	target := filepath.Join(s.dataDir, "workers", id, filename)
	raw, err := os.ReadFile(target)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return FileContent{}, ErrFileNotFound
		}
		return FileContent{}, err
	}

	mimeType := detectFileMimeType(filename, raw)
	if strings.HasPrefix(mimeType, "image/") {
		return FileContent{
			Filename:       filename,
			MediaType:      "image",
			MIMEType:       mimeType,
			PreviewDataURL: "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(raw),
		}, nil
	}
	if !utf8.Valid(raw) {
		return FileContent{}, ErrBinaryFileNotSupport
	}
	return FileContent{
		Filename:  filename,
		Content:   string(raw),
		MediaType: "text",
		MIMEType:  mimeType,
	}, nil
}

func (s *WorkerService) SaveWorkerFileContent(ctx context.Context, id string, filename string, content string) ([]string, error) {
	filename = strings.TrimSpace(filename)
	if err := validateFilename(filename); err != nil {
		return nil, err
	}
	if _, err := s.GetWorker(ctx, id); err != nil {
		return nil, err
	}

	workerDir := filepath.Join(s.dataDir, "workers", id)
	if err := os.MkdirAll(workerDir, 0o755); err != nil {
		return nil, fmt.Errorf("创建 Worker 目录失败: %w", err)
	}
	target := filepath.Join(workerDir, filename)
	if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
		return nil, fmt.Errorf("写入文件失败: %w", err)
	}
	return listFiles(workerDir)
}

func (s *WorkerService) UploadWorkerFiles(ctx context.Context, id string, fileHeaders []*multipart.FileHeader) ([]string, error) {
	if _, err := s.GetWorker(ctx, id); err != nil {
		return nil, err
	}
	if len(fileHeaders) == 0 {
		return nil, fmt.Errorf("至少上传一个文件")
	}

	workerDir := filepath.Join(s.dataDir, "workers", id)
	if err := os.MkdirAll(workerDir, 0o755); err != nil {
		return nil, fmt.Errorf("创建 Worker 目录失败: %w", err)
	}
	for _, fh := range fileHeaders {
		if err := saveUploadedFile(workerDir, fh); err != nil {
			return nil, err
		}
	}
	return listFiles(workerDir)
}

func (s *WorkerService) DeleteWorkerFile(ctx context.Context, id string, filename string) error {
	filename = strings.TrimSpace(filename)
	if err := validateFilename(filename); err != nil {
		return err
	}

	worker, err := s.GetWorker(ctx, id)
	if err != nil {
		return err
	}
	if filename == mainFilenameByRuntime(worker.Runtime) {
		return ErrMainFileDeletion
	}

	target := filepath.Join(s.dataDir, "workers", id, filename)
	if err := os.Remove(target); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ErrFileNotFound
		}
		return fmt.Errorf("删除文件失败: %w", err)
	}
	return nil
}

func (s *WorkerService) reloadWorkersState(ctx context.Context) error {
	if s.reg != nil {
		funcs, err := s.store.Worker.ListEnabled(ctx)
		if err != nil {
			return fmt.Errorf("加载启用函数失败: %w", err)
		}
		s.reg.Reload(funcs)
	}
	if s.cronReloader != nil {
		if err := s.cronReloader.Reload(ctx); err != nil {
			return fmt.Errorf("重载 cron 调度器失败: %w", err)
		}
	}
	return nil
}

func validateFilename(filename string) error {
	if filename == "" {
		return fmt.Errorf("filename 不能为空")
	}
	if filepath.Base(filename) != filename || strings.Contains(filename, "/") || strings.Contains(filename, "\\") {
		return ErrInvalidFilename
	}
	return nil
}

func detectFileMimeType(filename string, raw []byte) string {
	extMime := mime.TypeByExtension(strings.ToLower(filepath.Ext(filename)))
	if extMime != "" {
		return extMime
	}
	if len(raw) == 0 {
		return "application/octet-stream"
	}
	return http.DetectContentType(raw)
}

func saveUploadedFile(workerDir string, fh *multipart.FileHeader) error {
	if fh == nil {
		return fmt.Errorf("文件不能为空")
	}
	if err := validateFilename(fh.Filename); err != nil {
		return fmt.Errorf("非法文件名: %s", fh.Filename)
	}
	src, err := fh.Open()
	if err != nil {
		return fmt.Errorf("打开上传文件失败: %w", err)
	}
	defer src.Close()

	target := filepath.Join(workerDir, fh.Filename)
	dst, err := os.Create(target)
	if err != nil {
		return fmt.Errorf("写入文件失败: %w", err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return fmt.Errorf("保存文件失败: %w", err)
	}
	return nil
}

func mainFilenameByRuntime(runtime string) string {
	switch runtime {
	case "python":
		return "main.py"
	case "node":
		return "main.js"
	default:
		return ""
	}
}

func createMainFileFromTemplate(workerDir string, runtime string) error {
	mainFile := mainFilenameByRuntime(runtime)
	templateFile := templateFilenameByRuntime(runtime)
	if mainFile == "" || templateFile == "" {
		return fmt.Errorf("runtime 不合法")
	}

	templatePath, err := resolveTemplatePath(templateFile)
	if err != nil {
		return err
	}
	content, err := os.ReadFile(templatePath)
	if err != nil {
		return fmt.Errorf("读取模板文件失败: %w", err)
	}
	target := filepath.Join(workerDir, mainFile)
	if err := os.WriteFile(target, content, 0o644); err != nil {
		return fmt.Errorf("写入主文件失败: %w", err)
	}
	return nil
}

func resolveTemplatePath(templateFile string) (string, error) {
	candidates := []string{
		filepath.Join("resources", "worker_templates", templateFile),
		filepath.Join("..", "..", "resources", "worker_templates", templateFile),
		filepath.Join("..", "resources", "worker_templates", templateFile),
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("读取模板文件失败: 未找到模板 %s", templateFile)
}

func templateFilenameByRuntime(runtime string) string {
	switch runtime {
	case "python":
		return "python.py"
	case "node":
		return "node.js"
	default:
		return ""
	}
}

func listFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []string{}, nil
		}
		return nil, err
	}
	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		files = append(files, entry.Name())
	}
	sort.Strings(files)
	return files, nil
}
