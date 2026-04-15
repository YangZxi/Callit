package migrate

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"callit/internal/config"
	"callit/internal/db"
	"callit/internal/model"
	workerpkg "callit/internal/worker"
)

type Service struct {
	store *db.Store
	cfg   config.Config
}

func NewService(store *db.Store, cfg config.Config) *Service {
	return &Service{
		store: store,
		cfg:   cfg,
	}
}

func (s *Service) CleanupRemovedChatArtifacts(ctx context.Context) error {
	if s.store != nil && s.store.AppConfig != nil {
		removedChatAppConfigKeys := []string{
			"AI_BASE_URL",
			"AI_API_KEY",
			"AI_MODEL",
			"AI_MAX_CONTEXT_TOKENS",
			"AI_TIMEOUT_MS",
		}
		if err := s.store.AppConfig.DeleteConfigs(ctx, removedChatAppConfigKeys); err != nil {
			return fmt.Errorf("删除废弃 AI 配置项失败: %w", err)
		}
	}

	chatSessionsDir := filepath.Join(s.cfg.DataDir, "chat-sessions")
	if err := os.RemoveAll(chatSessionsDir); err != nil {
		return fmt.Errorf("删除 chat_sessions 目录失败: %w", err)
	}
	return nil
}

func (s *Service) RebuildWorkerDirStructure(ctx context.Context) error {
	entries, err := os.ReadDir(s.cfg.WorkersDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("读取 workers 目录失败: %w", err)
	}

	workerDirs := make([]string, 0, len(entries))
	needBackup := false
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		workerID := strings.TrimSpace(entry.Name())
		if workerID == "" {
			continue
		}
		workerDirs = append(workerDirs, workerID)
		isLegacy, err := s.isLegacyWorkerDir(filepath.Join(s.cfg.WorkersDir, workerID))
		if err != nil {
			return err
		}
		if isLegacy {
			needBackup = true
		}
	}

	if needBackup {
		if err := s.backupWorkersDir(); err != nil {
			return err
		}
	}

	for _, workerID := range workerDirs {
		if err := s.migrateWorker(ctx, workerID); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) migrateWorker(ctx context.Context, workerID string) error {
	rootDir := filepath.Join(s.cfg.WorkersDir, workerID)
	isLegacy, err := s.isLegacyWorkerDir(rootDir)
	if err != nil {
		return err
	}
	worker, err := s.loadWorker(ctx, workerID)
	if err != nil {
		return err
	}

	spec := workerpkg.NewWorkerSpec(s.cfg.WorkersDir, s.cfg.RuntimeLibDir, worker)
	if err := spec.EnsureLayout(); err != nil {
		return fmt.Errorf("创建 Worker 目录结构失败[%s]: %w", workerID, err)
	}
	if isLegacy {
		entries, err := os.ReadDir(rootDir)
		if err != nil {
			return fmt.Errorf("读取旧版 Worker 目录失败[%s]: %w", workerID, err)
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			if entry.Name() == "metadata.json" {
				continue
			}
			sourcePath := filepath.Join(rootDir, entry.Name())
			targetPath := filepath.Join(spec.WorkerCodeDir, entry.Name())
			if _, err := os.Stat(targetPath); err == nil {
				continue
			} else if err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("检查迁移目标失败[%s/%s]: %w", workerID, entry.Name(), err)
			}
			if err := os.Rename(sourcePath, targetPath); err != nil {
				return fmt.Errorf("迁移 Worker 文件失败[%s/%s]: %w", workerID, entry.Name(), err)
			}
		}
	}

	if worker.ID != "" {
		if err := spec.WriteMetadata(); err != nil {
			return fmt.Errorf("写入 metadata 失败[%s]: %w", workerID, err)
		}
	}
	return nil
}

func (s *Service) loadWorker(ctx context.Context, workerID string) (model.Worker, error) {
	if s.store == nil || s.store.Worker == nil {
		return model.Worker{ID: workerID}, nil
	}
	worker, err := s.store.Worker.GetByID(ctx, workerID)
	if err != nil {
		if db.IsNotFound(err) {
			return model.Worker{ID: workerID}, nil
		}
		return model.Worker{}, fmt.Errorf("读取 Worker 数据失败[%s]: %w", workerID, err)
	}
	return worker, nil
}

func (s *Service) isLegacyWorkerDir(rootDir string) (bool, error) {
	codeInfo, err := os.Stat(filepath.Join(rootDir, "code"))
	if err == nil && codeInfo.IsDir() {
		return false, nil
	}
	if err != nil && !os.IsNotExist(err) {
		return false, fmt.Errorf("检查 code 目录失败[%s]: %w", rootDir, err)
	}

	entries, err := os.ReadDir(rootDir)
	if err != nil {
		return false, fmt.Errorf("读取 Worker 目录失败[%s]: %w", rootDir, err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		switch entry.Name() {
		case "main.py", "main.js":
			return true, nil
		default:
			return true, nil
		}
	}
	return false, nil
}

func (s *Service) backupWorkersDir() error {
	entries, err := os.ReadDir(s.cfg.WorkerRunningTempDir)
	if err == nil {
		for _, entry := range entries {
			if entry.IsDir() && strings.HasPrefix(entry.Name(), "workers-migration-") {
				return nil
			}
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("读取临时目录失败: %w", err)
	}

	backupRoot := filepath.Join(s.cfg.WorkerRunningTempDir, "workers-migration-"+time.Now().Format("20060102-150405"))
	backupTarget := filepath.Join(backupRoot, "workers")
	if err := copyDir(s.cfg.WorkersDir, backupTarget); err != nil {
		return fmt.Errorf("备份 workers 目录失败: %w", err)
	}
	return nil
}

func copyDir(sourceDir string, targetDir string) error {
	return filepath.Walk(sourceDir, func(sourcePath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relativePath, err := filepath.Rel(sourceDir, sourcePath)
		if err != nil {
			return err
		}
		targetPath := filepath.Join(targetDir, relativePath)
		if info.IsDir() {
			return os.MkdirAll(targetPath, 0o755)
		}
		return copyFile(sourcePath, targetPath, info.Mode())
	})
}

func copyFile(sourcePath string, targetPath string, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return err
	}
	source, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer source.Close()

	target, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	defer target.Close()

	_, err = io.Copy(target, source)
	return err
}
