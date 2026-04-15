package router

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"callit/internal/common"
	"callit/internal/common/requestparse"
	"callit/internal/config"
	"callit/internal/db"
	"callit/internal/model"
	workerpkg "callit/internal/worker"
	"callit/internal/worker/executor"

	"github.com/gin-gonic/gin"
)

// Server 表示 Router 服务。
type Server struct {
	store     *db.Store
	reg       *Registry
	workerDir string
	invoker   workerInvoker
}

type workerInvoker interface {
	WorkerRunningTempDir() string
	Execute(ctx context.Context, worker model.Worker, requestID string, workerRunningTempDir string, input model.WorkerInput) executor.ExecuteResult
}

// NewEngine 创建 Router Gin 引擎。
func NewEngine(store *db.Store, reg *Registry, cfg config.Config, invoker workerInvoker) *gin.Engine {
	s := &Server{
		store:     store,
		reg:       reg,
		workerDir: cfg.WorkersDir,
		invoker:   invoker,
	}
	e := gin.New()
	e.Use(gin.Recovery(), common.RequestIDMiddleware())
	e.NoRoute(s.handleInvoke)
	return e
}

func (s *Server) handleInvoke(c *gin.Context) {
	requestID := common.GetRequestID(c)
	path := c.Request.URL.Path
	method := c.Request.Method
	contentType := c.GetHeader("Content-Type")

	matched := s.reg.Match(path)
	if !matched.Found {
		if path == "/" {
			c.String(http.StatusOK, "Hello Callit!")
			return
		}
		c.String(http.StatusNotFound, "404 NotFound")
		return
	}
	worker := matched.Worker

	rawBody, err := io.ReadAll(c.Request.Body)
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "读取请求体失败")
		return
	}

	workerRunningTempDir, cleanup, err := executor.CreateWorkerRunningTempDir(s.invoker.WorkerRunningTempDir(), requestID)
	defer cleanup()
	if err != nil {
		common.ErrorResponse(c, http.StatusInternalServerError, "创建运行时目录失败")
		return
	}
	parsed, err := s.parseRequetBodyToJson(contentType, rawBody, workerRunningTempDir)
	if err != nil {
		slog.Error("解析请求体失败", "request_id", requestID, "content_type", contentType, "err", err)
		common.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	requestBody := string(rawBody)
	if shouldStripRawBody(contentType) {
		requestBody = ""
	}

	headers := make(map[string]string, len(c.Request.Header))
	for key, vals := range c.Request.Header {
		headers[key] = strings.Join(vals, ",")
	}

	input := model.WorkerInput{
		Request: model.WorkerRequest{
			Method:  method,
			URI:     buildRouteSuffix(worker.Route, c.Request.URL.Path),
			URL:     buildFullURL(c.Request),
			Params:  buildQueryParams(c.Request.URL),
			Headers: headers,
			Body:    parsed,
			BodyStr: requestBody,
		},
		Event: model.WorkerEvent{
			RequestID: requestID,
			Trigger:   model.WorkerTriggerHTTP,
			Runtime:   worker.Runtime,
			WorkerID:  worker.ID,
			Route:     worker.Route,
		},
	}

	timeoutCtx, cancel := context.WithTimeout(c.Request.Context(), time.Duration(worker.TimeoutMS)*time.Millisecond)
	defer cancel()

	workerSpec, err := workerpkg.NewRuntimeWorkerSpec(s.workerDir, s.invoker.WorkerRunningTempDir(), "", worker, requestID)
	if err != nil {
		common.ErrorResponse(c, http.StatusInternalServerError, "构造 Worker 运行目录失败")
		return
	}
	workerDir := workerSpec.WorkerCodeDir
	execResult := s.invoker.Execute(timeoutCtx, worker, requestID, workerRunningTempDir, input)

	if execResult.TimedOut {
		slog.Warn("Worker 执行超时", "request_id", requestID, "worker_id", worker.ID, "timeout_ms", worker.TimeoutMS)
		common.ErrorResponse(c, http.StatusGatewayTimeout, "execution timeout")
		return
	}
	if execResult.Err != nil {
		common.ErrorResponse(c, http.StatusInternalServerError, execResult.Err.Error())
		return
	}

	if strings.TrimSpace(execResult.File) != "" {
		if err := writeByFile(c, workerDir, workerRunningTempDir, execResult.Status, execResult.Headers, execResult.File); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				common.ErrorResponse(c, http.StatusNotFound, "Worker中的资源不存在")
				return
			}
			common.ErrorResponse(c, http.StatusInternalServerError, "读取Worker资源失败")
			return
		}
		return
	}
	writeByBody(c, execResult.Status, execResult.Headers, execResult.Body)
}

// writeByBody 按 body 模式写回响应。
func writeByBody(c *gin.Context, status int, headers map[string]string, body any) {
	for key, val := range headers {
		c.Header(key, val)
	}

	if body == nil {
		if status == http.StatusNoContent || status == http.StatusNotModified {
			c.Status(status)
			return
		}
		c.JSON(status, nil)
		return
	}

	switch v := body.(type) {
	case string:
		contentType := c.Writer.Header().Get("Content-Type")
		if contentType == "" {
			contentType = "text/plain; charset=utf-8"
		}
		c.Data(status, contentType, []byte(v))
	default:
		c.JSON(status, body)
	}
}

// writeByFile 按 file 模式写回响应，支持从 Worker 目录或运行时目录读取文件。
func writeByFile(c *gin.Context, workerDir string, workerRunningTempDir string, status int, headers map[string]string, relativePath string) error {
	filePath, err := getRealFilePathFromResult(workerDir, workerRunningTempDir, relativePath)
	if err != nil {
		return err
	}
	stat, err := os.Stat(filePath)
	if err != nil {
		return err
	}
	if stat.IsDir() {
		return fs.ErrNotExist
	}
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	for key, val := range headers {
		c.Header(key, val)
	}
	if c.Writer.Header().Get("Content-Type") == "" {
		contentType := mime.TypeByExtension(filepath.Ext(filePath))
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		c.Header("Content-Type", contentType)
	}
	c.Status(status)

	if _, err := io.Copy(c.Writer, file); err != nil {
		return err
	}
	return nil
}

func shouldStripRawBody(contentType string) bool {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err == nil {
		return strings.EqualFold(mediaType, "multipart/form-data")
	}
	normalized := strings.ToLower(strings.TrimSpace(contentType))
	return strings.HasPrefix(normalized, "multipart/form-data")
}

func buildFullURL(req *http.Request) string {
	if req == nil || req.URL == nil {
		return ""
	}

	scheme := headerFirstValue(req.Header.Get("X-Forwarded-Proto"))
	host := headerFirstValue(req.Header.Get("X-Forwarded-Host"))

	if scheme == "" {
		scheme = strings.TrimSpace(req.URL.Scheme)
	}
	if scheme == "" {
		if req.TLS != nil {
			scheme = "https"
		} else {
			scheme = "http"
		}
	}
	if host == "" {
		host = strings.TrimSpace(req.Host)
	}
	if host == "" {
		host = strings.TrimSpace(req.URL.Host)
	}

	u := *req.URL
	u.Scheme = scheme
	u.Host = host
	return u.String()
}

func headerFirstValue(raw string) string {
	if raw == "" {
		return ""
	}
	first := strings.TrimSpace(strings.Split(raw, ",")[0])
	return first
}

// parseRequetBodyToJson 按请求 Content-Type 解析请求体。
func (s *Server) parseRequetBodyToJson(contentType string, rawBody []byte, workerRunningTempDir string) (parsed any, err error) {
	parsed = map[string]any{}

	mediaType, _, _ := mime.ParseMediaType(contentType)
	mediaType = strings.ToLower(mediaType)
	uploadDir := ""
	if shouldStripRawBody(contentType) {
		uploadDir = filepath.Join(workerRunningTempDir, "upload")
		slog.Debug("准备 multipart 上传目录", "path", uploadDir)
	}

	switch {
	case mediaType == "application/json":
		parsed, err = requestparse.ParseJSON(rawBody)
	case mediaType == "application/x-www-form-urlencoded":
		parsed, err = requestparse.ParseURLEncoded(rawBody)
	case shouldStripRawBody(contentType):
		parsed, err = requestparse.ParseMultipart(rawBody, contentType, uploadDir)
	default:
		parsed = map[string]any{}
	}
	return parsed, err
}

// getRealFilePathFromResult 将 result.file 解析为 Worker 目录或运行时目录中的绝对路径。
func getRealFilePathFromResult(workerDir string, workerRunningTempDir string, rawPath string) (string, error) {
	targetPath := strings.TrimSpace(rawPath)
	targetPath = strings.ReplaceAll(targetPath, "\\", "/")
	if targetPath == "" {
		return "", fs.ErrNotExist
	}

	realFilePath := ""
	if strings.HasPrefix(targetPath, "/tmp") {
		realFilePath = filepath.Join(workerRunningTempDir, strings.TrimLeft(strings.TrimPrefix(targetPath, "/tmp"), "/"))
	} else {
		if after, ok := strings.CutPrefix(targetPath, "./"); ok {
			targetPath = after
		}
		if after, ok := strings.CutPrefix(targetPath, "/"); ok {
			targetPath = after
		}
		realFilePath = filepath.Join(workerDir, targetPath)
	}
	slog.Debug("解析 Worker 返回文件路径", "raw_path", rawPath, "real_path", realFilePath)

	return realFilePath, nil
}

func buildRouteSuffix(route string, requestPath string) string {
	if parsed, err := url.Parse(requestPath); err == nil && parsed.Path != "" {
		requestPath = parsed.Path
	}

	if strings.HasSuffix(route, "/*") {
		prefix := strings.TrimSuffix(route, "*")
		if strings.HasPrefix(requestPath, prefix) {
			suffix := strings.TrimPrefix(requestPath, prefix)
			if suffix == "" {
				return "/"
			}
			return "/" + suffix
		}
	}
	return "/"
}

func buildQueryParams(requestURL *url.URL) map[string]string {
	if requestURL == nil {
		return map[string]string{}
	}

	values := requestURL.Query()
	params := make(map[string]string, len(values))
	for key, vals := range values {
		if len(vals) == 0 {
			params[key] = ""
			continue
		}
		params[key] = vals[len(vals)-1]
	}
	return params
}
