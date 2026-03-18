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
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"callit/internal/admin/chat"
	"callit/internal/common"
	"callit/internal/config"
	"callit/internal/db"
	"callit/internal/model"
	"callit/internal/registry"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const adminAuthCookieName = "callit_admin_token"

const (
	defaultWorkerLogsPage     = 1
	defaultWorkerLogsPageSize = 20
	maxWorkerLogsPageSize     = 100
)

// Server 表示 Admin 服务。
type Server struct {
	store       *db.Store
	reg         *registry.Registry
	dataDir     string
	adminToken  string
	chatHandler *chat.Handler
	configMu    sync.RWMutex

	dependencyTaskMu      sync.Mutex
	dependencyTaskRunning bool
}

type createWorkerRequest struct {
	Name      string `json:"name"`
	Runtime   string `json:"runtime"`
	Route     string `json:"route"`
	TimeoutMS int    `json:"timeout_ms"`
	Enabled   *bool  `json:"enabled"`
}

type loginRequest struct {
	Token string `json:"token"`
}

type saveFileRequest struct {
	Content string `json:"content"`
}

type renameFileRequest struct {
	Filename    string `json:"filename"`
	NewFilename string `json:"new_filename"`
}

func writeAPIResponse(c *gin.Context, httpStatus int, code int, msg string, data any) {
	c.JSON(httpStatus, gin.H{
		"code": code,
		"msg":  msg,
		"data": data,
	})
}

func apiSuccess(c *gin.Context, data any) {
	writeAPIResponse(c, http.StatusOK, 200, "ok", data)
}

func apiError(c *gin.Context, httpStatus int, msg string) {
	writeAPIResponse(c, httpStatus, httpStatus, msg, nil)
}

// NewEngine 创建 Admin Gin 引擎。
func NewEngine(store *db.Store, reg *registry.Registry, cfg config.Config) *gin.Engine {
	s := &Server{
		store:       store,
		reg:         reg,
		dataDir:     cfg.DataDir,
		adminToken:  cfg.AdminToken,
		chatHandler: chat.NewHandler(store, cfg.DataDir, cfg.AppConfig),
	}
	e := gin.New()
	e.Use(gin.Recovery(), common.RequestIDMiddleware())

	authAPI := e.Group(cfg.AdminPrefix + "/api/auth")
	{
		authAPI.GET("/status", s.authStatus)
		authAPI.POST("/login", s.login)
		authAPI.POST("/logout", s.logout)
	}

	api := e.Group(cfg.AdminPrefix + "/api")
	api.Use(s.authMiddleware())
	{
		api.GET("/dependencies", s.listDependencies)
		api.POST("/dependencies/manage", s.manageDependencies)

		api.POST("/workers", s.createWorker)
		api.PUT("/workers/:id", s.updateWorker)
		api.GET("/workers", s.listWorkers)
		api.GET("/workers/:id", s.getWorker)
		api.GET("/workers/:id/logs", s.listWorkerLogs)
		api.DELETE("/workers/:id", s.deleteWorker)
		api.POST("/workers/:id/files", s.uploadFiles)
		api.GET("/workers/:id/files", s.listWorkerFiles)
		api.GET("/workers/:id/files/content", s.getFileContent)
		api.PUT("/workers/:id/files/content", s.saveFileContent)
		api.DELETE("/workers/:id/files", s.deleteFile)
		api.POST("/workers/:id/files/rename", s.renameFile)
		api.POST("/workers/:id/enable", s.enableWorker)
		api.POST("/workers/:id/disable", s.disableWorker)
		api.GET("/workers/:id/chat/session", s.chatHandler.GetSession)
		api.POST("/workers/:id/chat/stream", s.chatHandler.Stream)
		api.POST("/workers/:id/chat/session/clear", s.chatHandler.ClearSession)

		api.GET("/config", s.AdminGetConfigHandler(&cfg))
		api.POST("/config", s.AdminUpsertConfigHandler(&cfg))
	}

	// 静态资源
	e.Static(cfg.AdminPrefix+"/assets", "./public/admin/assets")
	e.Static(cfg.AdminPrefix+"/static", "./public/admin/static")
	if cfg.AdminPrefix != "/admin" {
		e.Static("/admin/assets", "./public/admin/assets")
	}

	e.LoadHTMLFiles("public/admin/index.html")
	e.NoRoute(func(c *gin.Context) {
		path := c.Request.URL.Path
		if isAdminAPIPath(path, cfg.AdminPrefix) {
			apiError(c, http.StatusNotFound, "404 NotFound")
			return
		}
		// 如果设置了 AdminPrefix，那么需要拦截除了 /admin/assets 以外的请求
		if !strings.HasPrefix(path, cfg.AdminPrefix+"/") && !strings.HasPrefix(path, "/admin/assets") {
			apiError(c, http.StatusNotFound, "404 NotFound")
			return
		}

		c.HTML(http.StatusOK, "index.html", gin.H{
			"base_prefix": cfg.AdminPrefix,
		})
	})
	return e
}

func isAdminAPIPath(path string, adminPrefix string) bool {
	return path == adminPrefix+"/api" || path == adminPrefix+"/api/" || strings.HasPrefix(path, adminPrefix+"/api/")
}

func publicFilePathFromAdminPath(path string, adminPrefix string) string {
	trimmed := strings.TrimPrefix(path, adminPrefix)
	if trimmed == "" || trimmed == "/" {
		return "public/admin"
	}
	cleaned := strings.TrimPrefix(filepath.Clean(trimmed), "/")
	if cleaned == "." || cleaned == "" {
		return "public/admin"
	}
	return filepath.Join("public/admin", cleaned)
}

func (s *Server) authMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if s.isAuthorized(c) {
			c.Next()
			return
		}
		apiError(c, http.StatusUnauthorized, "unauthorized")
		c.Abort()
	}
}

func (s *Server) isAuthorized(c *gin.Context) bool {
	if token, err := c.Cookie(adminAuthCookieName); err == nil && token == s.adminToken {
		return true
	}

	authorization := strings.TrimSpace(c.GetHeader("Authorization"))
	const prefix = "Bearer "
	if !strings.HasPrefix(authorization, prefix) {
		return false
	}
	token := strings.TrimSpace(strings.TrimPrefix(authorization, prefix))
	return token != "" && token == s.adminToken
}

func (s *Server) authStatus(c *gin.Context) {
	apiSuccess(c, gin.H{
		"authenticated": s.isAuthorized(c),
	})
}

func (s *Server) login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiError(c, http.StatusBadRequest, "请求体格式错误")
		return
	}
	token := strings.TrimSpace(req.Token)
	if token == "" || token != s.adminToken {
		apiError(c, http.StatusUnauthorized, "token 无效")
		return
	}

	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(adminAuthCookieName, token, 7*24*3600, "/", "", false, true)
	apiSuccess(c, gin.H{"ok": true})
}

func (s *Server) logout(c *gin.Context) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(adminAuthCookieName, "", -1, "/", "", false, true)
	apiSuccess(c, gin.H{"ok": true})
}

func (s *Server) createWorker(c *gin.Context) {
	var req createWorkerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiError(c, http.StatusBadRequest, "请求体格式错误")
		return
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	if req.TimeoutMS <= 0 {
		req.TimeoutMS = 5000
	}

	worker := model.Worker{
		ID:        uuid.NewString(),
		Name:      strings.TrimSpace(req.Name),
		Runtime:   strings.TrimSpace(req.Runtime),
		Route:     strings.TrimSpace(req.Route),
		TimeoutMS: req.TimeoutMS,
		Enabled:   enabled,
	}
	if err := validateWorker(worker); err != nil {
		apiError(c, http.StatusBadRequest, err.Error())
		return
	}

	created, err := s.store.Worker.Create(c.Request.Context(), worker)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed: worker.route") {
			apiError(c, http.StatusConflict, "路由已存在")
			return
		}
		apiError(c, http.StatusInternalServerError, err.Error())
		return
	}

	functionDir := filepath.Join(s.dataDir, "workers", created.ID)
	if err := os.MkdirAll(functionDir, 0o755); err != nil {
		_ = s.store.Worker.Delete(context.Background(), created.ID)
		apiError(c, http.StatusInternalServerError, fmt.Sprintf("创建函数目录失败: %v", err))
		return
	}
	if err := createMainFileFromTemplate(functionDir, created.Runtime); err != nil {
		_ = os.RemoveAll(functionDir)
		_ = s.store.Worker.Delete(context.Background(), created.ID)
		apiError(c, http.StatusInternalServerError, fmt.Sprintf("创建入口文件失败: %v", err))
		return
	}

	if err := s.reloadRegistry(c.Request.Context()); err != nil {
		apiError(c, http.StatusInternalServerError, err.Error())
		return
	}
	apiSuccess(c, created)
}

func (s *Server) updateWorker(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		apiError(c, http.StatusBadRequest, "id 不能为空")
		return
	}

	origin, err := s.store.Worker.GetByID(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			apiError(c, http.StatusNotFound, "函数不存在")
			return
		}
		apiError(c, http.StatusInternalServerError, err.Error())
		return
	}

	var req createWorkerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiError(c, http.StatusBadRequest, "请求体格式错误")
		return
	}

	if req.TimeoutMS <= 0 {
		req.TimeoutMS = 5000
	}

	enabled := origin.Enabled
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	runtime := strings.TrimSpace(req.Runtime)
	if runtime == "" {
		runtime = origin.Runtime
	}
	if runtime != origin.Runtime {
		apiError(c, http.StatusBadRequest, "更新时不允许修改 runtime")
		return
	}

	updating := model.Worker{
		ID:        id,
		Name:      strings.TrimSpace(req.Name),
		Runtime:   runtime,
		Route:     strings.TrimSpace(req.Route),
		TimeoutMS: req.TimeoutMS,
		Enabled:   enabled,
	}
	if err := validateWorker(updating); err != nil {
		apiError(c, http.StatusBadRequest, err.Error())
		return
	}

	updated, err := s.store.Worker.Update(c.Request.Context(), updating)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			apiError(c, http.StatusNotFound, "函数不存在")
			return
		}
		if strings.Contains(err.Error(), "UNIQUE constraint failed: worker.route") {
			apiError(c, http.StatusConflict, "路由已存在")
			return
		}
		apiError(c, http.StatusInternalServerError, err.Error())
		return
	}

	if err := s.reloadRegistry(c.Request.Context()); err != nil {
		apiError(c, http.StatusInternalServerError, err.Error())
		return
	}
	apiSuccess(c, updated)
}

func (s *Server) listWorkers(c *gin.Context) {
	list, err := s.store.Worker.List(c.Request.Context())
	if err != nil {
		apiError(c, http.StatusInternalServerError, err.Error())
		return
	}
	apiSuccess(c, list)
}

func (s *Server) getWorker(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		apiError(c, http.StatusBadRequest, "id 不能为空")
		return
	}
	worker, err := s.store.Worker.GetByID(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			apiError(c, http.StatusNotFound, "函数不存在")
			return
		}
		apiError(c, http.StatusInternalServerError, err.Error())
		return
	}
	apiSuccess(c, worker)
}

func (s *Server) listWorkerLogs(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		apiError(c, http.StatusBadRequest, "id 不能为空")
		return
	}
	if _, err := s.store.Worker.GetByID(c.Request.Context(), id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			apiError(c, http.StatusNotFound, "函数不存在")
			return
		}
		apiError(c, http.StatusInternalServerError, err.Error())
		return
	}

	page := defaultWorkerLogsPage
	rawPage := strings.TrimSpace(c.Query("page"))
	if rawPage != "" {
		n, err := strconv.Atoi(rawPage)
		if err != nil || n <= 0 {
			apiError(c, http.StatusBadRequest, "page 必须是正整数")
			return
		}
		page = n
	}

	pageSize := defaultWorkerLogsPageSize
	rawPageSize := strings.TrimSpace(c.Query("page_size"))
	if rawPageSize != "" {
		n, err := strconv.Atoi(rawPageSize)
		if err != nil || n <= 0 {
			apiError(c, http.StatusBadRequest, "page_size 必须是正整数")
			return
		}
		if n > maxWorkerLogsPageSize {
			apiError(c, http.StatusBadRequest, fmt.Sprintf("page_size 不能超过 %d", maxWorkerLogsPageSize))
			return
		}
		pageSize = n
	}

	logs, total, err := s.store.WorkerLog.ListPaged(c.Request.Context(), id, page, pageSize)
	if err != nil {
		apiError(c, http.StatusInternalServerError, err.Error())
		return
	}
	apiSuccess(c, gin.H{
		"page":      page,
		"page_size": pageSize,
		"total":     total,
		"data":      logs,
	})
}

func (s *Server) deleteWorker(c *gin.Context) {
	id := c.Param("id")
	if strings.TrimSpace(id) == "" {
		apiError(c, http.StatusBadRequest, "id 不能为空")
		return
	}
	if err := s.store.Worker.Delete(c.Request.Context(), id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			apiError(c, http.StatusNotFound, "函数不存在")
			return
		}
		apiError(c, http.StatusInternalServerError, err.Error())
		return
	}

	functionDir := filepath.Join(s.dataDir, "workers", id)
	if err := os.RemoveAll(functionDir); err != nil {
		apiError(c, http.StatusInternalServerError, fmt.Sprintf("删除函数目录失败: %v", err))
		return
	}
	if err := s.reloadRegistry(c.Request.Context()); err != nil {
		apiError(c, http.StatusInternalServerError, err.Error())
		return
	}
	apiSuccess(c, gin.H{"ok": true})
}

func (s *Server) uploadFiles(c *gin.Context) {
	id := c.Param("id")
	worker, err := s.store.Worker.GetByID(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			apiError(c, http.StatusNotFound, "函数不存在")
			return
		}
		apiError(c, http.StatusInternalServerError, err.Error())
		return
	}

	if err := c.Request.ParseMultipartForm(64 << 20); err != nil {
		apiError(c, http.StatusBadRequest, fmt.Sprintf("解析上传内容失败: %v", err))
		return
	}
	if c.Request.MultipartForm != nil {
		defer c.Request.MultipartForm.RemoveAll()
	}
	if c.Request.MultipartForm == nil || len(c.Request.MultipartForm.File) == 0 {
		apiError(c, http.StatusBadRequest, "至少上传一个文件")
		return
	}

	functionDir := filepath.Join(s.dataDir, "workers", id)
	if err := os.MkdirAll(functionDir, 0o755); err != nil {
		apiError(c, http.StatusInternalServerError, fmt.Sprintf("创建函数目录失败: %v", err))
		return
	}

	fileHeaders := flattenFiles(c.Request.MultipartForm.File)
	if len(fileHeaders) == 0 {
		apiError(c, http.StatusBadRequest, "至少上传一个文件")
		return
	}

	for _, fh := range fileHeaders {
		if err := saveUploadedFile(functionDir, fh); err != nil {
			apiError(c, http.StatusBadRequest, err.Error())
			return
		}
	}

	if err := ensureMainFileExists(functionDir, worker.Runtime); err != nil {
		apiError(c, http.StatusBadRequest, err.Error())
		return
	}

	files, err := listFiles(functionDir)
	if err != nil {
		apiError(c, http.StatusInternalServerError, err.Error())
		return
	}
	apiSuccess(c, gin.H{"files": files})
}

func (s *Server) listWorkerFiles(c *gin.Context) {
	id := c.Param("id")
	if _, err := s.store.Worker.GetByID(c.Request.Context(), id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			apiError(c, http.StatusNotFound, "函数不存在")
			return
		}
		apiError(c, http.StatusInternalServerError, err.Error())
		return
	}

	functionDir := filepath.Join(s.dataDir, "workers", id)
	if err := os.MkdirAll(functionDir, 0o755); err != nil {
		apiError(c, http.StatusInternalServerError, fmt.Sprintf("创建函数目录失败: %v", err))
		return
	}
	files, err := listFiles(functionDir)
	if err != nil {
		apiError(c, http.StatusInternalServerError, err.Error())
		return
	}
	apiSuccess(c, gin.H{"files": files})
}

func (s *Server) getFileContent(c *gin.Context) {
	id := c.Param("id")
	filename := strings.TrimSpace(c.Query("filename"))
	if filename == "" {
		apiError(c, http.StatusBadRequest, "filename 不能为空")
		return
	}
	if filepath.Base(filename) != filename || strings.Contains(filename, "/") || strings.Contains(filename, "\\") {
		apiError(c, http.StatusBadRequest, "非法文件名")
		return
	}

	if _, err := s.store.Worker.GetByID(c.Request.Context(), id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			apiError(c, http.StatusNotFound, "函数不存在")
			return
		}
		apiError(c, http.StatusInternalServerError, err.Error())
		return
	}

	path := filepath.Join(s.dataDir, "workers", id, filename)
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			apiError(c, http.StatusNotFound, "文件不存在")
			return
		}
		apiError(c, http.StatusInternalServerError, err.Error())
		return
	}

	mimeType := detectFileMimeType(filename, raw)
	if strings.HasPrefix(mimeType, "image/") {
		dataURL := "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(raw)
		apiSuccess(c, gin.H{
			"filename":         filename,
			"content":          "",
			"media_type":       "image",
			"mime_type":        mimeType,
			"preview_data_url": dataURL,
		})
		return
	}

	if !utf8.Valid(raw) {
		apiError(c, http.StatusBadRequest, "仅支持文本文件和图片预览")
		return
	}

	apiSuccess(c, gin.H{
		"filename":   filename,
		"content":    string(raw),
		"media_type": "text",
		"mime_type":  mimeType,
	})
}

func (s *Server) saveFileContent(c *gin.Context) {
	id := c.Param("id")
	filename := strings.TrimSpace(c.Query("filename"))
	if filename == "" {
		apiError(c, http.StatusBadRequest, "filename 不能为空")
		return
	}
	if filepath.Base(filename) != filename || strings.Contains(filename, "/") || strings.Contains(filename, "\\") {
		apiError(c, http.StatusBadRequest, "非法文件名")
		return
	}

	worker, err := s.store.Worker.GetByID(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			apiError(c, http.StatusNotFound, "函数不存在")
			return
		}
		apiError(c, http.StatusInternalServerError, err.Error())
		return
	}

	var req saveFileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiError(c, http.StatusBadRequest, "请求体格式错误")
		return
	}

	functionDir := filepath.Join(s.dataDir, "workers", id)
	if err := os.MkdirAll(functionDir, 0o755); err != nil {
		apiError(c, http.StatusInternalServerError, fmt.Sprintf("创建函数目录失败: %v", err))
		return
	}

	mainFile := mainFilenameByRuntime(worker.Runtime)
	if filename != mainFile {
		if _, err := os.Stat(filepath.Join(functionDir, mainFile)); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				apiError(c, http.StatusBadRequest, fmt.Sprintf("请先创建主文件 %s", mainFile))
				return
			}
			apiError(c, http.StatusInternalServerError, err.Error())
			return
		}
	}

	target := filepath.Join(functionDir, filename)
	if err := os.WriteFile(target, []byte(req.Content), 0o644); err != nil {
		apiError(c, http.StatusInternalServerError, fmt.Sprintf("写入文件失败: %v", err))
		return
	}

	files, err := listFiles(functionDir)
	if err != nil {
		apiError(c, http.StatusInternalServerError, err.Error())
		return
	}
	apiSuccess(c, gin.H{"files": files})
}

func (s *Server) deleteFile(c *gin.Context) {
	id := c.Param("id")
	filename := strings.TrimSpace(c.Query("filename"))
	if filename == "" {
		apiError(c, http.StatusBadRequest, "filename 不能为空")
		return
	}
	if filepath.Base(filename) != filename || strings.Contains(filename, "/") || strings.Contains(filename, "\\") {
		apiError(c, http.StatusBadRequest, "非法文件名")
		return
	}

	worker, err := s.store.Worker.GetByID(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			apiError(c, http.StatusNotFound, "函数不存在")
			return
		}
		apiError(c, http.StatusInternalServerError, err.Error())
		return
	}

	mainFile := mainFilenameByRuntime(worker.Runtime)
	if filename == mainFile {
		apiError(c, http.StatusBadRequest, "入口文件不能删除")
		return
	}

	target := filepath.Join(s.dataDir, "workers", id, filename)
	if err := os.Remove(target); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			apiError(c, http.StatusNotFound, "文件不存在")
			return
		}
		apiError(c, http.StatusInternalServerError, fmt.Sprintf("删除文件失败: %v", err))
		return
	}

	if err := ensureMainFileExists(filepath.Join(s.dataDir, "workers", id), worker.Runtime); err != nil {
		apiError(c, http.StatusBadRequest, err.Error())
		return
	}
	apiSuccess(c, gin.H{"ok": true})
}

func (s *Server) renameFile(c *gin.Context) {
	id := c.Param("id")

	var req renameFileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiError(c, http.StatusBadRequest, "请求体格式错误")
		return
	}

	filename := strings.TrimSpace(req.Filename)
	newFilename := strings.TrimSpace(req.NewFilename)
	if filename == "" || newFilename == "" {
		apiError(c, http.StatusBadRequest, "filename 和 new_filename 不能为空")
		return
	} else if filename == newFilename {
		apiError(c, http.StatusBadRequest, "新文件名不能与原文件名相同")
		return
	}
	if filepath.Base(filename) != filename || strings.Contains(filename, "/") || strings.Contains(filename, "\\") {
		apiError(c, http.StatusBadRequest, "非法文件名")
		return
	}
	if filepath.Base(newFilename) != newFilename || strings.Contains(newFilename, "/") || strings.Contains(newFilename, "\\") {
		apiError(c, http.StatusBadRequest, "非法文件名")
		return
	}

	worker, err := s.store.Worker.GetByID(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			apiError(c, http.StatusNotFound, "函数不存在")
			return
		}
		apiError(c, http.StatusInternalServerError, err.Error())
		return
	}

	mainFile := mainFilenameByRuntime(worker.Runtime)
	if filename == mainFile || newFilename == mainFile {
		apiError(c, http.StatusBadRequest, "入口文件不能重命名")
		return
	}

	functionDir := filepath.Join(s.dataDir, "workers", id)
	if err := renameWorkerFile(functionDir, filename, newFilename); err != nil {
		switch {
		case errors.Is(err, errSourceFileNotExist):
			apiError(c, http.StatusNotFound, "文件不存在")
		case errors.Is(err, errTargetFileExists):
			apiError(c, http.StatusConflict, "文件名已存在")
		default:
			apiError(c, http.StatusInternalServerError, fmt.Sprintf("重命名文件失败: %v", err))
		}
		return
	}

	files, err := listFiles(functionDir)
	if err != nil {
		apiError(c, http.StatusInternalServerError, err.Error())
		return
	}
	apiSuccess(c, gin.H{"files": files, "filename": newFilename})
}

func (s *Server) enableWorker(c *gin.Context) {
	s.setWorkerEnabled(c, true)
}

func (s *Server) disableWorker(c *gin.Context) {
	s.setWorkerEnabled(c, false)
}

func (s *Server) setWorkerEnabled(c *gin.Context, enabled bool) {
	id := c.Param("id")
	updated, err := s.store.Worker.SetEnabled(c.Request.Context(), id, enabled)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			apiError(c, http.StatusNotFound, "函数不存在")
			return
		}
		apiError(c, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.reloadRegistry(c.Request.Context()); err != nil {
		apiError(c, http.StatusInternalServerError, err.Error())
		return
	}
	apiSuccess(c, updated)
}

func (s *Server) reloadRegistry(ctx context.Context) error {
	reloadCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	funcs, err := s.store.Worker.ListEnabled(reloadCtx)
	if err != nil {
		return fmt.Errorf("加载启用函数失败: %w", err)
	}
	s.reg.Reload(funcs)
	return nil
}

func validateWorker(worker model.Worker) error {
	if strings.TrimSpace(worker.Name) == "" {
		return fmt.Errorf("name 不能为空")
	}
	if worker.Runtime != "python" && worker.Runtime != "node" {
		return fmt.Errorf("runtime 仅支持 python 或 node")
	}
	if err := model.ValidateRoute(worker.Route); err != nil {
		return err
	}
	if worker.TimeoutMS <= 0 {
		return fmt.Errorf("timeout_ms 必须大于 0")
	}
	return nil
}

func flattenFiles(fileMap map[string][]*multipart.FileHeader) []*multipart.FileHeader {
	result := make([]*multipart.FileHeader, 0)
	for _, files := range fileMap {
		result = append(result, files...)
	}
	return result
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

func saveUploadedFile(functionDir string, fh *multipart.FileHeader) error {
	if fh == nil {
		return fmt.Errorf("文件不能为空")
	}
	name := fh.Filename
	if filepath.Base(name) != name || strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return fmt.Errorf("非法文件名: %s", name)
	}
	src, err := fh.Open()
	if err != nil {
		return fmt.Errorf("打开上传文件失败: %w", err)
	}
	defer src.Close()

	target := filepath.Join(functionDir, name)
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

func ensureMainFileExists(functionDir string, runtime string) error {
	mainFile := mainFilenameByRuntime(runtime)
	if mainFile == "" {
		return fmt.Errorf("runtime 不合法")
	}
	if _, err := os.Stat(filepath.Join(functionDir, mainFile)); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("主文件缺失，必须包含 %s", mainFile)
		}
		return err
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

func createMainFileFromTemplate(functionDir string, runtime string) error {
	mainFile := mainFilenameByRuntime(runtime)
	if mainFile == "" {
		return fmt.Errorf("runtime 不合法")
	}
	templateFile := templateFilenameByRuntime(runtime)
	if templateFile == "" {
		return fmt.Errorf("runtime 不合法")
	}
	templatePath := filepath.Join("resources/worker_templates", templateFile)
	content, err := os.ReadFile(templatePath)
	if err != nil {
		return fmt.Errorf("读取模板文件失败: %w", err)
	}
	target := filepath.Join(functionDir, mainFile)
	if err := os.WriteFile(target, content, 0o644); err != nil {
		return fmt.Errorf("写入主文件失败: %w", err)
	}
	return nil
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

var (
	errSourceFileNotExist = errors.New("source file not exist")
	errTargetFileExists   = errors.New("target file already exists")
)

func renameWorkerFile(functionDir string, filename string, newFilename string) error {
	sourcePath := filepath.Join(functionDir, filename)
	if _, err := os.Stat(sourcePath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return errSourceFileNotExist
		}
		return err
	}

	targetPath := filepath.Join(functionDir, newFilename)
	if _, err := os.Stat(targetPath); err == nil {
		return errTargetFileExists
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	return os.Rename(sourcePath, targetPath)
}
