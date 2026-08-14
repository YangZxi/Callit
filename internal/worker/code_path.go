package worker

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"
)

var ErrInvalidCodePath = errors.New("invalid worker code path")

const (
	maxLinuxFilenameBytes = 255
	maxLinuxPathBytes     = 4095
)

type CodePath struct {
	RelativePath string
	IsDirectory  bool
}

// ParseCodePath 将前端和 MCP 使用的斜杠路径转换为受控的 Worker 相对路径。
func ParseCodePath(raw string, maxDirectoryDepth int) (CodePath, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || maxDirectoryDepth <= 0 {
		return CodePath{}, fmt.Errorf("%w: 路径不能为空", ErrInvalidCodePath)
	}
	if strings.Contains(raw, "\\") {
		return CodePath{}, fmt.Errorf("%w: 路径不能包含反斜杠", ErrInvalidCodePath)
	}

	isDirectory := strings.HasSuffix(raw, "/")
	trimmed := strings.TrimLeft(raw, "/")
	if isDirectory {
		trimmed = strings.TrimSuffix(trimmed, "/")
	}
	if trimmed == "" {
		return CodePath{}, fmt.Errorf("%w: 路径不能为空", ErrInvalidCodePath)
	}
	if path.IsAbs(trimmed) || filepath.IsAbs(trimmed) || isWindowsAbsolutePath(trimmed) {
		return CodePath{}, fmt.Errorf("%w: 路径必须位于 Worker 目录内", ErrInvalidCodePath)
	}

	rawSegments := strings.Split(trimmed, "/")
	segments := make([]string, 0, len(rawSegments))
	for _, rawSegment := range rawSegments {
		segment := strings.TrimSpace(rawSegment)
		switch {
		case segment == "":
			return CodePath{}, fmt.Errorf("%w: 路径不能包含空名称", ErrInvalidCodePath)
		case segment == "." || segment == "..":
			return CodePath{}, fmt.Errorf("%w: 路径不能包含 . 或 ..", ErrInvalidCodePath)
		case !utf8.ValidString(segment):
			return CodePath{}, fmt.Errorf("%w: 路径必须是有效的 UTF-8", ErrInvalidCodePath)
		case len([]byte(segment)) > maxLinuxFilenameBytes:
			return CodePath{}, fmt.Errorf("%w: 单个名称不能超过 %d 字节", ErrInvalidCodePath, maxLinuxFilenameBytes)
		case strings.IndexFunc(segment, unicode.IsControl) >= 0:
			return CodePath{}, fmt.Errorf("%w: 路径不能包含控制字符", ErrInvalidCodePath)
		}
		segments = append(segments, segment)
	}
	normalized := strings.Join(segments, "/")
	if path.Clean(normalized) != normalized {
		return CodePath{}, fmt.Errorf("%w: 路径格式错误", ErrInvalidCodePath)
	}
	if len([]byte(normalized)) > maxLinuxPathBytes {
		return CodePath{}, fmt.Errorf("%w: 路径不能超过 %d 字节", ErrInvalidCodePath, maxLinuxPathBytes)
	}

	directoryDepth := len(segments)
	if isDirectory {
		directoryDepth++
	}
	if directoryDepth > maxDirectoryDepth {
		return CodePath{}, fmt.Errorf("%w: 目录层级不能超过 %d 层", ErrInvalidCodePath, maxDirectoryDepth)
	}

	return CodePath{RelativePath: normalized, IsDirectory: isDirectory}, nil
}

func ResolveCodePath(rootDir string, codePath CodePath) (string, error) {
	root, err := filepath.Abs(rootDir)
	if err != nil {
		return "", err
	}
	target := filepath.Join(root, filepath.FromSlash(codePath.RelativePath))
	relative, err := filepath.Rel(root, target)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", ErrInvalidCodePath
	}
	return target, nil
}

// EnsureCodePathHasNoSymlink 拒绝通过 Worker 目录中的软链接访问目录外内容。
// allowMissing 为 true 时，在遇到首个不存在的路径段后停止检查，供新建文件使用。
func EnsureCodePathHasNoSymlink(rootDir string, codePath CodePath, allowMissing bool) error {
	current := rootDir
	for _, segment := range strings.Split(codePath.RelativePath, "/") {
		current = filepath.Join(current, segment)
		info, err := os.Lstat(current)
		if err != nil {
			if allowMissing && errors.Is(err, os.ErrNotExist) {
				return nil
			}
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return ErrInvalidCodePath
		}
	}
	return nil
}

func isWindowsAbsolutePath(value string) bool {
	return len(value) >= 3 && value[1] == ':' && value[2] == '/'
}
