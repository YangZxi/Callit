package worker

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"callit/internal/model"
)

const (
	codeDirName  = "code"
	dataDirName  = "data"
	metadataName = "metadata.json"
)

type WorkerSpec struct {
	Worker        model.Worker
	WorkerRootDir string
	WorkerCodeDir string
	WorkerDataDir string
	WorkerTempDir string
	RuntimeLibDir string
	MetadataPath  string
}

func NewWorkerSpec(workersDir string, runtimeLibDir string, worker model.Worker) WorkerSpec {
	rootDir := filepath.Join(workersDir, worker.ID)
	return WorkerSpec{
		Worker:        worker,
		WorkerRootDir: rootDir,
		WorkerCodeDir: filepath.Join(rootDir, codeDirName),
		WorkerDataDir: filepath.Join(rootDir, dataDirName),
		RuntimeLibDir: runtimeLibDir,
		MetadataPath:  filepath.Join(rootDir, metadataName),
	}
}

func NewRuntimeWorkerSpec(workersDir string, workerTempBaseDir string, runtimeLibDir string, worker model.Worker, requestID string) (WorkerSpec, error) {
	if requestID == "" {
		return WorkerSpec{}, errors.New("requestID 不能为空")
	}
	spec := NewWorkerSpec(workersDir, runtimeLibDir, worker)
	spec.WorkerTempDir = filepath.Join(workerTempBaseDir, requestID)
	return spec, nil
}

func (s WorkerSpec) MainFilename() string {
	switch s.Worker.Runtime {
	case "python":
		return "main.py"
	case "node":
		return "main.js"
	default:
		return ""
	}
}

func (s WorkerSpec) EnsureLayout() error {
	for _, dir := range []string{s.WorkerRootDir, s.WorkerCodeDir, s.WorkerDataDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return nil
}

func (s WorkerSpec) WriteMetadata() error {
	if err := s.EnsureLayout(); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(s.Worker, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化 metadata 失败: %w", err)
	}
	raw = append(raw, '\n')
	return writeFileAtomically(s.MetadataPath, raw, 0o644)
}

func (s WorkerSpec) ListCodeFiles() ([]string, error) {
	return ListFiles(s.WorkerCodeDir)
}

func (s WorkerSpec) CodeFilePath(filename string) string {
	return filepath.Join(s.WorkerCodeDir, filename)
}

func ListFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
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

func RenameCodeFile(dir string, filename string, newFilename string) error {
	sourcePath := filepath.Join(dir, filename)
	targetPath := filepath.Join(dir, newFilename)

	if _, err := os.Stat(sourcePath); err != nil {
		if os.IsNotExist(err) {
			return ErrSourceFileNotExist
		}
		return err
	}
	if _, err := os.Stat(targetPath); err == nil {
		return ErrTargetFileExists
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.Rename(sourcePath, targetPath)
}

func WriteCodeFile(dir string, filename string, content []byte) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, filename), content, 0o644)
}

type namedError string

func (e namedError) Error() string { return string(e) }

var (
	ErrSourceFileNotExist = namedError("source file not exist")
	ErrTargetFileExists   = namedError("target file exists")
)

func DeletedWorkerRootDir(workersDir string, workerID string) string {
	return filepath.Join(workersDir, "deleted_"+workerID)
}

func SoftDeleteWorkerRootDir(workersDir string, workerID string) error {
	sourcePath := filepath.Join(workersDir, workerID)
	if _, err := os.Stat(sourcePath); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	targetPath := DeletedWorkerRootDir(workersDir, workerID)
	if _, err := os.Stat(targetPath); err == nil {
		return fmt.Errorf("软删除目标目录已存在: %s", targetPath)
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.Rename(sourcePath, targetPath)
}

func writeFileAtomically(path string, content []byte, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, content, perm); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}
