package router

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"callit/internal/common/requestparse"

	"github.com/gin-gonic/gin"
)

func TestBuildQueryParams(t *testing.T) {
	u, err := url.Parse("http://100.64.0.7:3100/api/js?data=123&tag=a&tag=b&empty=")
	if err != nil {
		t.Fatalf("parse url failed: %v", err)
	}

	got := buildQueryParams(u)
	want := map[string]string{
		"data":  "123",
		"tag":   "b",
		"empty": "",
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected params, got=%v want=%v", got, want)
	}
}

func TestBuildQueryParamsNilURL(t *testing.T) {
	got := buildQueryParams(nil)
	if len(got) != 0 {
		t.Fatalf("expected empty params map, got=%v", got)
	}
}

func TestShouldStripRawBody(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		want        bool
	}{
		{
			name:        "multipart带boundary",
			contentType: "multipart/form-data; boundary=----WebKitFormBoundaryabc",
			want:        true,
		},
		{
			name:        "multipart无boundary也应识别",
			contentType: "multipart/form-data",
			want:        true,
		},
		{
			name:        "json不应清空body",
			contentType: "application/json",
			want:        false,
		},
		{
			name:        "空content-type不应清空body",
			contentType: "",
			want:        false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := shouldStripRawBody(tc.contentType)
			if got != tc.want {
				t.Fatalf("unexpected shouldStripRawBody result, got=%v want=%v contentType=%q", got, tc.want, tc.contentType)
			}
		})
	}
}

func TestBuildFullURL(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://example.com/tea/123?x=1", nil)
	got := buildFullURL(req)
	want := "http://example.com/tea/123?x=1"
	if got != want {
		t.Fatalf("unexpected full url, got=%s want=%s", got, want)
	}
}

func TestBuildFullURLForwardedHeaders(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://internal.local/tea/123?x=1", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-Host", "api.example.com")

	got := buildFullURL(req)
	want := "https://api.example.com/tea/123?x=1"
	if got != want {
		t.Fatalf("unexpected full url with forwarded headers, got=%s want=%s", got, want)
	}
}

func TestBuildRouteSuffix(t *testing.T) {
	tests := []struct {
		name        string
		route       string
		requestPath string
		want        string
	}{
		{
			name:        "基础路径无斜杠",
			route:       "/tea/*",
			requestPath: "/tea",
			want:        "/",
		},
		{
			name:        "基础路径带斜杠",
			route:       "/tea/*",
			requestPath: "/tea/",
			want:        "/",
		},
		{
			name:        "子路径",
			route:       "/tea/*",
			requestPath: "/tea/a",
			want:        "/a",
		},
		{
			name:        "基础路径带查询串",
			route:       "/tea/*",
			requestPath: "/tea/?token=1",
			want:        "/",
		},
		{
			name:        "子路径带查询串",
			route:       "/tea/*",
			requestPath: "/tea/a?token=1",
			want:        "/a",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := buildRouteSuffix(tc.route, tc.requestPath)
			if got != tc.want {
				t.Fatalf("unexpected route suffix, got=%q want=%q", got, tc.want)
			}
		})
	}
}

func TestRequestPathDoesNotIncludeQuery(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/tea?token=1", nil)
	if req.URL.Path != "/tea" {
		t.Fatalf("unexpected request path, got=%q want=%q", req.URL.Path, "/tea")
	}

	params := buildQueryParams(req.URL)
	want := map[string]string{"token": "1"}
	if !reflect.DeepEqual(params, want) {
		t.Fatalf("unexpected query params, got=%v want=%v", params, want)
	}
}

func TestParseRequetBodyToJsonStoresMultipartFilesUnderWorkerRunningTempDir(t *testing.T) {
	workerRunningTempDir := t.TempDir()
	workerRunningTempDirUpload := filepath.Join(workerRunningTempDir, "req-1")
	s := &Server{}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("title", "demo"); err != nil {
		t.Fatalf("写入表单字段失败: %v", err)
	}
	fileWriter, err := writer.CreateFormFile("file", "hello.txt")
	if err != nil {
		t.Fatalf("创建上传文件字段失败: %v", err)
	}
	if _, err := fileWriter.Write([]byte("hello world")); err != nil {
		t.Fatalf("写入上传文件失败: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("关闭 multipart writer 失败: %v", err)
	}

	parsed, err := s.parseRequetBodyToJson(writer.FormDataContentType(), body.Bytes(), workerRunningTempDirUpload)
	if err != nil {
		t.Fatalf("解析 multipart 请求失败: %v", err)
	}

	form, ok := parsed.(map[string]any)
	if !ok {
		t.Fatalf("解析结果类型不正确: %#v", parsed)
	}
	files, ok := form["file"].([]requestparse.FileMeta)
	if !ok || len(files) != 1 {
		t.Fatalf("期望 file 字段包含 1 个文件元数据: %#v", form["file"])
	}
	meta := files[0]
	wantPath := filepath.ToSlash(filepath.Join("/tmp", "upload", "hello.txt"))
	if meta.Path != wantPath {
		t.Fatalf("上传文件路径不正确，got=%v want=%v", meta.Path, wantPath)
	}
	if _, err := os.Stat(filepath.Join(workerRunningTempDirUpload, "upload", "hello.txt")); err != nil {
		t.Fatalf("上传文件未落盘: %v", err)
	}
}

func TestGetRealFilePathFromResultSupportsWorkerDir(t *testing.T) {
	workerDir := t.TempDir()
	targetPath := filepath.Join(workerDir, "static", "hello.txt")
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		t.Fatalf("创建 worker 目录失败: %v", err)
	}
	if err := os.WriteFile(targetPath, []byte("worker"), 0o644); err != nil {
		t.Fatalf("写入 worker 文件失败: %v", err)
	}

	got, err := getRealFilePathFromResult(workerDir, "", "static/hello.txt")
	if err != nil {
		t.Fatalf("解析 worker 文件路径失败: %v", err)
	}
	if got != targetPath {
		t.Fatalf("worker 文件路径不正确，got=%q want=%q", got, targetPath)
	}
}

func TestGetRealFilePathFromResultSupportsWorkerRunningTempDir(t *testing.T) {
	workerDir := t.TempDir()
	workerRunningTempDir := t.TempDir()
	targetPath := filepath.Join(workerRunningTempDir, "upload", "hello.txt")
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		t.Fatalf("创建运行时目录失败: %v", err)
	}
	if err := os.WriteFile(targetPath, []byte("runtime"), 0o644); err != nil {
		t.Fatalf("写入运行时文件失败: %v", err)
	}

	got, err := getRealFilePathFromResult(workerDir, workerRunningTempDir, "/tmp/upload/hello.txt")
	if err != nil {
		t.Fatalf("解析运行时文件路径失败: %v", err)
	}
	if got != targetPath {
		t.Fatalf("运行时文件路径不正确，got=%q want=%q", got, targetPath)
	}
}

func TestGetRealFilePathFromResultReplacesTmpPrefix(t *testing.T) {
	workerDir := t.TempDir()
	workerRunningTempDir := t.TempDir()
	targetPath := filepath.Join(workerRunningTempDir, "-output.txt")
	if err := os.WriteFile(targetPath, []byte("runtime"), 0o644); err != nil {
		t.Fatalf("写入运行时文件失败: %v", err)
	}

	got, err := getRealFilePathFromResult(workerDir, workerRunningTempDir, "/tmp-output.txt")
	if err != nil {
		t.Fatalf("解析运行时文件路径失败: %v", err)
	}
	if got != targetPath {
		t.Fatalf("运行时文件路径不正确，got=%q want=%q", got, targetPath)
	}
}

func TestGetRealFilePathFromResultDoesNotCheckTargetMissing(t *testing.T) {
	workerDir := t.TempDir()
	want := filepath.Join(workerDir, "static", "missing.txt")

	got, err := getRealFilePathFromResult(workerDir, "", "static/missing.txt")
	if err != nil {
		t.Fatalf("目标不存在时不应报错，got=%v", err)
	}
	if got != want {
		t.Fatalf("目标不存在时路径不正确，got=%q want=%q", got, want)
	}
}

func TestGetRealFilePathFromResultDoesNotCheckTargetIsDir(t *testing.T) {
	workerDir := t.TempDir()
	targetDir := filepath.Join(workerDir, "static")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatalf("创建 worker 目录失败: %v", err)
	}

	got, err := getRealFilePathFromResult(workerDir, "", "static")
	if err != nil {
		t.Fatalf("目标为目录时不应报错，got=%v", err)
	}
	if got != targetDir {
		t.Fatalf("目标为目录时路径不正确，got=%q want=%q", got, targetDir)
	}
}

func TestWriteByFileSupportsWorkerRunningTempDir(t *testing.T) {
	workerDir := t.TempDir()
	workerRunningTempDir := t.TempDir()
	targetPath := filepath.Join(workerRunningTempDir, "upload", "hello.txt")
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		t.Fatalf("创建运行时目录失败: %v", err)
	}
	if err := os.WriteFile(targetPath, []byte("runtime"), 0o644); err != nil {
		t.Fatalf("写入运行时文件失败: %v", err)
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)

	if err := writeByFile(ctx, workerDir, workerRunningTempDir, http.StatusOK, map[string]string{}, "/tmp/upload/hello.txt"); err != nil {
		t.Fatalf("写回运行时文件失败: %v", err)
	}

	resp := recorder.Result()
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("读取响应体失败: %v", err)
	}
	if string(body) != "runtime" {
		t.Fatalf("响应体不正确，got=%q want=%q", string(body), "runtime")
	}
}

func TestWriteByFileReturnsNotExistWhenTargetMissing(t *testing.T) {
	workerDir := t.TempDir()

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)

	err := writeByFile(ctx, workerDir, "", http.StatusOK, map[string]string{}, "static/missing.txt")
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("目标不存在时应返回 fs.ErrNotExist，got=%v", err)
	}
}

func TestWriteByFileReturnsNotExistWhenTargetIsDir(t *testing.T) {
	workerDir := t.TempDir()
	targetDir := filepath.Join(workerDir, "static")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatalf("创建 worker 目录失败: %v", err)
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)

	err := writeByFile(ctx, workerDir, "", http.StatusOK, map[string]string{}, "static")
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("目标为目录时应返回 fs.ErrNotExist，got=%v", err)
	}
}
