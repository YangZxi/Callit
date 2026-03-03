package router

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"callit/internal/common"
	"callit/internal/db"
	"callit/internal/executor"
	"callit/internal/model"
	"callit/internal/registry"
	"callit/internal/requestparse"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Server 表示 Router 服务。
type Server struct {
	store   *db.Store
	reg     *registry.Registry
	dataDir string
}

// NewEngine 创建 Router Gin 引擎。
func NewEngine(store *db.Store, reg *registry.Registry, dataDir string) *gin.Engine {
	s := &Server{store: store, reg: reg, dataDir: dataDir}
	e := gin.New()
	e.Use(gin.Recovery(), common.RequestIDMiddleware())
	e.GET("/", func(c *gin.Context) {
		c.String(http.StatusOK, "Hello Callit!")
	})
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
		c.String(http.StatusNotFound, "404 NotFound")
		return
	}
	fn := matched.Worker

	rawBody, err := io.ReadAll(c.Request.Body)
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "读取请求体失败")
		return
	}

	parsed, cleanup, err := s.parseRequetBodyToJson(requestID, contentType, rawBody)
	defer cleanup()
	if err != nil {
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
			Method: method,
			URI:    buildRouteSuffix(fn.Route, c.Request.URL.RequestURI()),
			// todo, fix schema and host when behind proxy
			URL:     fmt.Sprintf("%s://%s%s", c.Request.URL.Scheme, c.Request.Host, c.Request.URL.String()),
			Params:  buildQueryParams(c.Request.URL),
			Headers: headers,
			Body:    requestBody,
			JSON:    parsed,
		},
		Event: model.WorkerEvent{
			RequestID: requestID,
			Runtime:   fn.Runtime,
			WorkerID:  fn.ID,
			Route:     fn.Route,
		},
	}

	timeoutCtx, cancel := context.WithTimeout(c.Request.Context(), time.Duration(fn.TimeoutMS)*time.Millisecond)
	defer cancel()

	workerDir := filepath.Join(s.dataDir, "workers", fn.ID)
	execResult := executor.Run(timeoutCtx, fn, workerDir, input)
	s.recordRunningLog(fn.ID, requestID, execResult)

	if execResult.TimedOut {
		common.ErrorResponse(c, http.StatusGatewayTimeout, "execution timeout")
		return
	}
	if execResult.Err != nil {
		common.ErrorResponse(c, http.StatusInternalServerError, execResult.Err.Error())
		return
	}

	if strings.TrimSpace(execResult.File) != "" {
		if err := writeByFile(c, workerDir, execResult.Status, execResult.Headers, execResult.File); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				common.ErrorResponse(c, http.StatusNotFound, "Worker资源不存在")
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

// writeByFile 按 file 模式写回响应。
func writeByFile(c *gin.Context, workerDir string, status int, headers map[string]string, relativePath string) error {
	filePath, err := getWorkerFilePath(workerDir, relativePath)
	if err != nil {
		return err
	}
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()
	stat, err := os.Stat(filePath)
	if err != nil {
		return err
	}
	if stat.IsDir() {
		return fs.ErrNotExist
	}

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

// parseRequetBodyToJson 按请求 Content-Type 解析请求体。
func (s *Server) parseRequetBodyToJson(requestID string, contentType string, rawBody []byte) (parsed any, cleanup func(), err error) {
	parsed = map[string]any{}
	cleanup = func() {}

	mediaType, _, _ := mime.ParseMediaType(contentType)
	mediaType = strings.ToLower(mediaType)
	uploadDir := ""
	if shouldStripRawBody(contentType) {
		uploadDir = filepath.Join(s.dataDir, "temps", requestID)
		cleanup = func() {
			if rmErr := os.RemoveAll(uploadDir); rmErr != nil {
				log.Printf("删除上传目录失败: %v", rmErr)
			}
		}
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
	return parsed, cleanup, err
}

// getWorkerFilePath result.file 的相对路径解析为绝对路径
func getWorkerFilePath(workerDir string, rawPath string) (string, error) {
	normalized := strings.TrimSpace(rawPath)
	normalized = strings.ReplaceAll(normalized, "\\", "/")
	normalized = strings.TrimLeft(normalized, "/")
	if normalized == "" {
		return "", fs.ErrNotExist
	}

	cleanPath := path.Clean(normalized)
	if cleanPath == "." || cleanPath == ".." || strings.HasPrefix(cleanPath, "../") {
		return "", fs.ErrNotExist
	}

	workerAbs, err := filepath.Abs(workerDir)
	if err != nil {
		return "", err
	}
	fileAbs, err := filepath.Abs(filepath.Join(workerAbs, filepath.FromSlash(cleanPath)))
	if err != nil {
		return "", err
	}

	if fileAbs != workerAbs && !strings.HasPrefix(fileAbs, workerAbs+string(os.PathSeparator)) {
		return "", fs.ErrNotExist
	}
	return fileAbs, nil
}

func buildRouteSuffix(route string, requestPath string) string {
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

func (s *Server) insertWorkerLogAsync(entry model.WorkerLog) {
	go func(logEntry model.WorkerLog) {
		persistCtx, persistCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer persistCancel()

		if err := s.store.InsertWorkerLog(persistCtx, logEntry); err != nil {
			log.Printf("写入函数日志失败: %v", err)
		}
	}(entry)
}

// recordRunningLog 统一整理执行结果并异步记录 Worker 运行日志。
func (s *Server) recordRunningLog(workerID string, requestID string, execResult executor.ExecuteResult) {
	statusForLog := execResult.Status
	if statusForLog == 0 {
		if execResult.TimedOut {
			statusForLog = http.StatusGatewayTimeout
		} else {
			statusForLog = http.StatusInternalServerError
		}
	}

	errMsg := ""
	if execResult.Err != nil {
		if execResult.TimedOut {
			errMsg = "timeout"
		} else {
			errMsg = execResult.Err.Error()
		}
	}

	s.insertWorkerLogAsync(model.WorkerLog{
		ID:         uuid.NewString(),
		WorkerID:   workerID,
		RequestID:  requestID,
		Status:     statusForLog,
		Stdout:     execResult.Stdout,
		Stderr:     execResult.Stderr,
		Error:      errMsg,
		DurationMS: execResult.DurationMS,
	})
}
