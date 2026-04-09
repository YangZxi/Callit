package executor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func SyncWorkerSDKFiles(runtimeLibDir string) error {
	resourceDir, err := findWorkerSDKResourceDir()
	if err != nil {
		return err
	}

	items := []struct {
		sourcePath string
		targetPath string
		isDir      bool
	}{
		{
			sourcePath: filepath.Join(resourceDir, "python", "callit"),
			targetPath: filepath.Join(runtimeLibDir, "python", "callit"),
			isDir:      true,
		},
		{
			sourcePath: filepath.Join(resourceDir, "node", "callit"),
			targetPath: filepath.Join(runtimeLibDir, "node", "node_modules", "callit"),
			isDir:      true,
		},
	}

	for _, item := range items {
		if item.isDir {
			if err := os.RemoveAll(item.targetPath); err != nil {
				return fmt.Errorf("清理旧 Worker SDK 目录失败: %w", err)
			}
		}
		var err error
		if item.isDir {
			err = copyWorkerSDKTree(item.sourcePath, item.targetPath)
		} else {
			err = copyWorkerSDKFile(item.sourcePath, item.targetPath)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func findWorkerSDKResourceDir() (string, error) {
	workingDir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("获取当前工作目录失败: %w", err)
	}

	currentDir := workingDir
	for {
		candidate := filepath.Join(currentDir, "resources", "worker_sdk")
		info, statErr := os.Stat(candidate)
		if statErr == nil && info.IsDir() {
			return candidate, nil
		}

		parentDir := filepath.Dir(currentDir)
		if parentDir == currentDir {
			break
		}
		currentDir = parentDir
	}
	return "", fmt.Errorf("未找到 resources/worker_sdk 目录")
}

func copyWorkerSDKFile(sourcePath string, targetPath string) error {
	content, err := os.ReadFile(sourcePath)
	if err != nil {
		return fmt.Errorf("读取 Worker SDK 模板失败: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return fmt.Errorf("创建 Worker SDK 目录失败: %w", err)
	}
	if err := os.WriteFile(targetPath, content, 0o644); err != nil {
		return fmt.Errorf("写入 Worker SDK 文件失败: %w", err)
	}
	return nil
}

func copyWorkerSDKTree(sourceRoot string, targetRoot string) error {
	return filepath.Walk(sourceRoot, func(sourcePath string, info os.FileInfo, err error) error {
		if err != nil {
			return fmt.Errorf("遍历 Worker SDK 模板失败: %w", err)
		}
		if info.IsDir() {
			return nil
		}

		relativePath := strings.TrimPrefix(sourcePath, sourceRoot+string(os.PathSeparator))
		targetPath := filepath.Join(targetRoot, filepath.FromSlash(relativePath))
		return copyWorkerSDKFile(sourcePath, targetPath)
	})
}
