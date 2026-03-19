package requestparse

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var (
	// ErrFieldConflict 表示同名字段同时作为普通字段和文件字段。
	ErrFieldConflict = errors.New("字段冲突：同名字段不能同时为普通字段和文件字段")
	bracketPattern   = regexp.MustCompile(`^([^\[\]]+)\[(\d*)\]$`)
)

type fieldKind int

const (
	fieldKindPlain fieldKind = iota + 1
	fieldKindBracket
	fieldKindFile
)

// FileMeta 是 multipart 文件元数据。
type FileMeta struct {
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	Size        int64  `json:"size"`
	Path        string `json:"path"`
}

type builder struct {
	data  map[string]any
	kinds map[string]fieldKind
}

func newBuilder() *builder {
	return &builder{
		data:  make(map[string]any),
		kinds: make(map[string]fieldKind),
	}
}

func (b *builder) addField(key string, value string) error {
	key = strings.TrimSpace(key)
	if key == "" {
		return fmt.Errorf("字段名不能为空")
	}

	if matches := bracketPattern.FindStringSubmatch(key); len(matches) == 3 {
		base := matches[1]
		indexRaw := matches[2]
		if kind, ok := b.kinds[base]; ok && kind != fieldKindBracket {
			return ErrFieldConflict
		}
		arr, _ := b.data[base].([]any)
		if indexRaw == "" {
			arr = append(arr, value)
			b.data[base] = arr
			b.kinds[base] = fieldKindBracket
			return nil
		}
		idx, err := strconv.Atoi(indexRaw)
		if err != nil || idx < 0 {
			return fmt.Errorf("非法数组下标: %s", key)
		}
		for len(arr) <= idx {
			arr = append(arr, nil)
		}
		arr[idx] = value
		b.data[base] = arr
		b.kinds[base] = fieldKindBracket
		return nil
	}

	if kind, ok := b.kinds[key]; ok {
		if kind == fieldKindFile || kind == fieldKindBracket {
			return ErrFieldConflict
		}
	}

	if existing, ok := b.data[key]; ok {
		switch cast := existing.(type) {
		case []any:
			b.data[key] = append(cast, value)
		default:
			b.data[key] = []any{cast, value}
		}
	} else {
		b.data[key] = value
	}
	b.kinds[key] = fieldKindPlain
	return nil
}

func (b *builder) addFile(key string, file FileMeta) error {
	key = strings.TrimSpace(key)
	if key == "" {
		return fmt.Errorf("文件字段名不能为空")
	}
	if kind, ok := b.kinds[key]; ok && kind != fieldKindFile {
		return ErrFieldConflict
	}
	var files []FileMeta
	if existing, ok := b.data[key]; ok {
		cast, ok := existing.([]FileMeta)
		if !ok {
			return ErrFieldConflict
		}
		files = cast
	}
	files = append(files, file)
	b.data[key] = files
	b.kinds[key] = fieldKindFile
	return nil
}

// ParseJSON 解析 application/json。
func ParseJSON(raw []byte) (any, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return map[string]any{}, nil
	}
	var parsed any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, err
	}
	return parsed, nil
}

// ParseURLEncoded 解析 x-www-form-urlencoded。
func ParseURLEncoded(raw []byte) (map[string]any, error) {
	if len(raw) == 0 {
		return map[string]any{}, nil
	}
	values, err := url.ParseQuery(string(raw))
	if err != nil {
		return nil, err
	}
	b := newBuilder()
	for key, list := range values {
		for _, v := range list {
			if err := b.addField(key, v); err != nil {
				return nil, err
			}
		}
	}
	return b.data, nil
}

// ParseMultipart 解析 multipart/form-data，并把文件写入 uploadDir。
func ParseMultipart(raw []byte, contentType string, uploadDir string) (map[string]any, error) {
	if err := os.MkdirAll(uploadDir, 0o755); err != nil {
		return nil, fmt.Errorf("创建上传目录失败: %w", err)
	}

	_, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return nil, fmt.Errorf("解析 Content-Type 失败: %w", err)
	}
	boundary := params["boundary"]
	if boundary == "" {
		return nil, fmt.Errorf("multipart 缺少 boundary")
	}

	reader := multipart.NewReader(bytes.NewReader(raw), boundary)
	b := newBuilder()
	usedFileNames := make(map[string]struct{})

	for {
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("读取 multipart 失败: %w", err)
		}

		name := part.FormName()
		if name == "" {
			part.Close()
			continue
		}

		fileName := part.FileName()
		if fileName == "" {
			val, readErr := io.ReadAll(part)
			part.Close()
			if readErr != nil {
				return nil, fmt.Errorf("读取普通字段失败: %w", readErr)
			}
			if err := b.addField(name, string(val)); err != nil {
				return nil, err
			}
			continue
		}

		safeName := filepath.Base(fileName)
		if safeName == "." || safeName == "/" || safeName == "" || strings.Contains(fileName, "/") || strings.Contains(fileName, "\\") {
			part.Close()
			return nil, fmt.Errorf("非法文件名: %s", fileName)
		}
		if _, ok := usedFileNames[safeName]; ok {
			part.Close()
			return nil, fmt.Errorf("同一请求内文件名重复: %s", safeName)
		}
		usedFileNames[safeName] = struct{}{}

		targetPath := filepath.Join(uploadDir, safeName)
		out, createErr := os.Create(targetPath)
		if createErr != nil {
			part.Close()
			return nil, fmt.Errorf("创建上传文件失败: %w", createErr)
		}
		size, copyErr := io.Copy(out, part)
		closeErr := out.Close()
		part.Close()
		if copyErr != nil {
			return nil, fmt.Errorf("写入上传文件失败: %w", copyErr)
		}
		if closeErr != nil {
			return nil, fmt.Errorf("关闭上传文件失败: %w", closeErr)
		}

		meta := FileMeta{
			Filename:    safeName,
			ContentType: part.Header.Get("Content-Type"),
			Size:        size,
			Path:        filepath.ToSlash(targetPath),
		}
		if meta.ContentType == "" {
			meta.ContentType = "application/octet-stream"
		}
		if err := b.addFile(name, meta); err != nil {
			return nil, err
		}
	}
	return b.data, nil
}
