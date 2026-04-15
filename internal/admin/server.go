package admin

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"callit/internal/admin/message"
	"callit/internal/common"
	"callit/internal/common/snowflake"
	"callit/internal/config"
	"callit/internal/cron"
	"callit/internal/db"
	"callit/internal/model"
	"callit/internal/router"
	workerpkg "callit/internal/worker"

	"github.com/gin-gonic/gin"
)

const adminAuthCookieName = "callit_admin_token"

const (
	defaultWorkerLogsPage     = 1
	defaultWorkerLogsPageSize = 20
	maxWorkerLogsPageSize     = 100
)

// Server 表示 Admin 服务。
type Server struct {
	store        *db.Store
	reg          *router.Registry
	cronReloader interface{ Reload(context.Context) error }
	idGenerator  *snowflake.Generator
	dataDir      string
	workersDir   string
	adminToken   string
	workerSvc    *WorkerService
	configMu     sync.RWMutex

	dependencyTaskMu      sync.Mutex
	dependencyTaskRunning bool
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
func NewEngine(store *db.Store, reg *router.Registry, cronReloader interface{ Reload(context.Context) error }, cfg *config.Config) *gin.Engine {
	workersDir := cfg.WorkersDir
	if strings.TrimSpace(workersDir) == "" {
		workersDir = filepath.Join(cfg.DataDir, "workers")
	}
	workerTmpDir := cfg.WorkerRunningTempDir
	if strings.TrimSpace(workerTmpDir) == "" {
		workerTmpDir = filepath.Join(cfg.DataDir, "tmp")
	}
	runtimeLibDir := cfg.RuntimeLibDir
	if strings.TrimSpace(runtimeLibDir) == "" {
		runtimeLibDir = filepath.Join(cfg.DataDir, ".lib")
	}
	s := &Server{
		store:        store,
		reg:          reg,
		cronReloader: cronReloader,
		idGenerator:  snowflake.NewGenerator(1),
		dataDir:      cfg.DataDir,
		workersDir:   workersDir,
		adminToken:   cfg.AdminToken,
		workerSvc:    NewWorkerService(store, reg, cronReloader, workersDir, workerTmpDir, runtimeLibDir),
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
		api.POST("/dependencies/rebuild", s.rebuildDependencies)

		api.GET("/workers", s.listWorkers)
		api.GET("/workers/:id", s.getWorker)
		api.POST("/workers/create", s.createWorker)
		api.POST("/workers/update", s.updateWorker)
		api.POST("/workers/delete", s.deleteWorker)
		api.POST("/workers/enable", s.enableWorker)
		api.POST("/workers/disable", s.disableWorker)

		api.GET("/workers/:id/logs", s.listWorkerLogs)
		api.GET("/workers/:id/crons", s.listWorkerCrons)
		api.POST("/workers/:id/crons/create", s.createWorkerCron)
		api.POST("/workers/:id/crons/update", s.updateWorkerCron)
		api.POST("/workers/:id/crons/delete", s.deleteWorkerCron)

		api.GET("/workers/:id/files", s.listWorkerFiles)
		api.GET("/workers/:id/files/:filename", s.getFileContent)
		api.POST("/workers/:id/files/upload", s.uploadFiles)
		api.POST("/workers/:id/files/update", s.saveFileContent)
		api.POST("/workers/:id/files/delete", s.deleteFile)
		api.POST("/workers/:id/files/rename", s.renameFile)

		api.GET("/config", s.AdminGetConfigHandler(cfg))
		api.POST("/config", s.AdminUpsertConfigHandler(cfg))
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
	var req message.LoginRequest
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
	var req message.CreateWorkerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiError(c, http.StatusBadRequest, "请求体格式错误")
		return
	}

	created, err := s.workerSvc.CreateWorker(c.Request.Context(), CreateWorkerInput{
		Name:        req.Name,
		Description: req.Description,
		Runtime:     req.Runtime,
		Route:       req.Route,
		TimeoutMS:   req.TimeoutMS,
		Env:         req.Env,
		Enabled:     req.Enabled,
	})
	if err != nil {
		if errors.Is(err, ErrWorkerRouteExists) {
			apiError(c, http.StatusConflict, "路由已存在")
			return
		}
		if strings.Contains(err.Error(), "不能为空") || strings.Contains(err.Error(), "仅支持") || strings.Contains(err.Error(), "必须") || strings.Contains(err.Error(), "不合法") || strings.Contains(err.Error(), "不能超过") {
			apiError(c, http.StatusBadRequest, err.Error())
			return
		}
		apiError(c, http.StatusInternalServerError, err.Error())
		return
	}
	apiSuccess(c, created)
}

func (s *Server) updateWorker(c *gin.Context) {
	var req struct {
		message.WorkerIDRequest
		message.UpdateWorkerRequest
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		apiError(c, http.StatusBadRequest, "请求体格式错误")
		return
	}

	id := strings.TrimSpace(req.ID)
	if id == "" {
		apiError(c, http.StatusBadRequest, "id 不能为空")
		return
	}

	updated, err := s.workerSvc.UpdateWorker(c.Request.Context(), UpdateWorkerInput{
		ID:          id,
		Name:        req.Name,
		Description: req.Description,
		Route:       req.Route,
		TimeoutMS:   req.TimeoutMS,
		Env:         req.Env,
		Enabled:     req.Enabled,
	})
	if err != nil {
		if errors.Is(err, ErrWorkerNotFound) {
			apiError(c, http.StatusNotFound, "函数不存在")
			return
		}
		if errors.Is(err, ErrWorkerRouteExists) {
			apiError(c, http.StatusConflict, "路由已存在")
			return
		}
		if strings.Contains(err.Error(), "不能为空") || strings.Contains(err.Error(), "仅支持") || strings.Contains(err.Error(), "必须") || strings.Contains(err.Error(), "不能超过") {
			apiError(c, http.StatusBadRequest, err.Error())
			return
		}
		apiError(c, http.StatusInternalServerError, err.Error())
		return
	}
	apiSuccess(c, updated)
}

func (s *Server) listWorkers(c *gin.Context) {
	keyword := strings.TrimSpace(c.Query("keyword"))
	list, err := s.workerSvc.SearchWorkers(c.Request.Context(), keyword)
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
	worker, err := s.workerSvc.GetWorker(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, ErrWorkerNotFound) {
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
	if _, err := s.workerSvc.GetWorker(c.Request.Context(), id); err != nil {
		if errors.Is(err, ErrWorkerNotFound) {
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
	var req message.WorkerIDRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiError(c, http.StatusBadRequest, "请求体格式错误")
		return
	}

	id := strings.TrimSpace(req.ID)
	if id == "" {
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
	if err := s.store.CronTask.DeleteByWorkerID(c.Request.Context(), id); err != nil {
		apiError(c, http.StatusInternalServerError, err.Error())
		return
	}

	if err := workerpkg.SoftDeleteWorkerRootDir(s.workersDir, id); err != nil {
		apiError(c, http.StatusInternalServerError, fmt.Sprintf("删除 Worker 失败: %v", err))
		return
	}
	if err := s.reloadWorkersState(c.Request.Context()); err != nil {
		apiError(c, http.StatusInternalServerError, err.Error())
		return
	}
	apiSuccess(c, gin.H{"ok": true})
}

func (s *Server) uploadFiles(c *gin.Context) {
	id := c.Param("id")
	if _, err := s.workerSvc.GetWorker(c.Request.Context(), id); err != nil {
		if errors.Is(err, ErrWorkerNotFound) {
			apiError(c, http.StatusNotFound, "Worker 不存在")
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

	fileHeaders := flattenFiles(c.Request.MultipartForm.File)
	files, err := s.workerSvc.UploadWorkerFiles(c.Request.Context(), id, fileHeaders)
	if err != nil {
		apiError(c, http.StatusBadRequest, err.Error())
		return
	}
	apiSuccess(c, gin.H{"files": files})
}

func (s *Server) listWorkerFiles(c *gin.Context) {
	id := c.Param("id")
	files, err := s.workerSvc.ListWorkerFiles(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, ErrWorkerNotFound) {
			apiError(c, http.StatusNotFound, "函数不存在")
			return
		}
		apiError(c, http.StatusInternalServerError, err.Error())
		return
	}
	apiSuccess(c, gin.H{"files": files})
}

func (s *Server) getFileContent(c *gin.Context) {
	id := c.Param("id")
	content, err := s.workerSvc.GetWorkerFile(c.Request.Context(), id, strings.TrimSpace(c.Param("filename")))
	if err != nil {
		switch {
		case errors.Is(err, ErrWorkerNotFound):
			apiError(c, http.StatusNotFound, "函数不存在")
		case errors.Is(err, ErrFileNotFound):
			apiError(c, http.StatusNotFound, "文件不存在")
		case errors.Is(err, ErrBinaryFileNotSupport):
			apiError(c, http.StatusBadRequest, "仅支持文本文件和图片预览")
		default:
			apiError(c, http.StatusBadRequest, err.Error())
		}
		return
	}
	apiSuccess(c, gin.H{
		"filename":         content.Filename,
		"content":          content.Content,
		"media_type":       content.MediaType,
		"mime_type":        content.MIMEType,
		"preview_data_url": content.PreviewDataURL,
	})
}

func (s *Server) saveFileContent(c *gin.Context) {
	id := c.Param("id")
	var req message.SaveFileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiError(c, http.StatusBadRequest, "请求体格式错误")
		return
	}

	files, err := s.workerSvc.SaveWorkerFileContent(c.Request.Context(), id, req.Filename, req.Content)
	if err != nil {
		switch {
		case errors.Is(err, ErrWorkerNotFound):
			apiError(c, http.StatusNotFound, "Worker 不存在")
		default:
			apiError(c, http.StatusBadRequest, err.Error())
		}
		return
	}
	apiSuccess(c, gin.H{"files": files})
}

func (s *Server) deleteFile(c *gin.Context) {
	id := c.Param("id")
	var req message.DeleteFileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiError(c, http.StatusBadRequest, "请求体格式错误")
		return
	}

	if err := s.workerSvc.DeleteWorkerFile(c.Request.Context(), id, req.Filename); err != nil {
		switch {
		case errors.Is(err, ErrWorkerNotFound):
			apiError(c, http.StatusNotFound, "Worker 不存在")
		case errors.Is(err, ErrMainFileDeletion):
			apiError(c, http.StatusBadRequest, "入口文件不能删除")
		case errors.Is(err, ErrFileNotFound):
			apiError(c, http.StatusNotFound, "文件不存在")
		default:
			apiError(c, http.StatusBadRequest, err.Error())
		}
		return
	}
	apiSuccess(c, gin.H{"ok": true})
}

func (s *Server) renameFile(c *gin.Context) {
	id := c.Param("id")

	var req message.RenameFileRequest
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

	functionDir := filepath.Join(s.workersDir, id, "code")
	if err := renameWorkerFile(functionDir, filename, newFilename); err != nil {
		switch {
		case errors.Is(err, workerpkg.ErrSourceFileNotExist):
			apiError(c, http.StatusNotFound, "文件不存在")
		case errors.Is(err, workerpkg.ErrTargetFileExists):
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
	var req message.WorkerIDRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiError(c, http.StatusBadRequest, "请求体格式错误")
		return
	}

	id := strings.TrimSpace(req.ID)
	if id == "" {
		apiError(c, http.StatusBadRequest, "id 不能为空")
		return
	}

	updated, err := s.workerSvc.SetWorkerEnabled(c.Request.Context(), id, enabled)
	if err != nil {
		if errors.Is(err, ErrWorkerNotFound) {
			apiError(c, http.StatusNotFound, "函数不存在")
			return
		}
		apiError(c, http.StatusInternalServerError, err.Error())
		return
	}
	apiSuccess(c, updated)
}

func (s *Server) reloadWorkersState(ctx context.Context) error {
	if err := s.reloadRegistry(ctx); err != nil {
		return err
	}
	if s.cronReloader != nil {
		if err := s.cronReloader.Reload(ctx); err != nil {
			return fmt.Errorf("重载 cron 调度器失败: %w", err)
		}
	}
	return nil
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

func (s *Server) listWorkerCrons(c *gin.Context) {
	workerID, ok := s.requireWorker(c)
	if !ok {
		return
	}

	tasks, err := s.store.CronTask.ListByWorkerID(c.Request.Context(), workerID)
	if err != nil {
		apiError(c, http.StatusInternalServerError, err.Error())
		return
	}
	apiSuccess(c, tasks)
}

func (s *Server) createWorkerCron(c *gin.Context) {
	workerID, ok := s.requireWorker(c)
	if !ok {
		return
	}

	var req message.CreateCronTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiError(c, http.StatusBadRequest, "请求体格式错误")
		return
	}

	cronExpr := strings.TrimSpace(req.Cron)
	if err := s.validateCronExpression(cronExpr); err != nil {
		apiError(c, http.StatusBadRequest, err.Error())
		return
	}

	taskID, err := s.idGenerator.NextID()
	if err != nil {
		apiError(c, http.StatusInternalServerError, err.Error())
		return
	}

	created, err := s.store.CronTask.Create(c.Request.Context(), model.CronTask{
		ID:       taskID,
		WorkerID: workerID,
		Cron:     cronExpr,
	})
	if err != nil {
		apiError(c, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.reloadWorkersState(c.Request.Context()); err != nil {
		apiError(c, http.StatusInternalServerError, err.Error())
		return
	}
	apiSuccess(c, created)
}

func (s *Server) updateWorkerCron(c *gin.Context) {
	workerID, ok := s.requireWorker(c)
	if !ok {
		return
	}

	var req message.UpdateCronTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiError(c, http.StatusBadRequest, "请求体格式错误")
		return
	}
	if req.ID <= 0 {
		apiError(c, http.StatusBadRequest, "id 不能为空")
		return
	}

	cronExpr := strings.TrimSpace(req.Cron)
	if err := s.validateCronExpression(cronExpr); err != nil {
		apiError(c, http.StatusBadRequest, err.Error())
		return
	}

	updated, err := s.store.CronTask.Update(c.Request.Context(), model.CronTask{
		ID:       req.ID,
		WorkerID: workerID,
		Cron:     cronExpr,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			apiError(c, http.StatusNotFound, "cron_task 不存在")
			return
		}
		apiError(c, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.reloadWorkersState(c.Request.Context()); err != nil {
		apiError(c, http.StatusInternalServerError, err.Error())
		return
	}
	apiSuccess(c, updated)
}

func (s *Server) deleteWorkerCron(c *gin.Context) {
	workerID, ok := s.requireWorker(c)
	if !ok {
		return
	}

	var req message.CronTaskIDRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiError(c, http.StatusBadRequest, "请求体格式错误")
		return
	}
	if req.ID <= 0 {
		apiError(c, http.StatusBadRequest, "id 不能为空")
		return
	}

	if err := s.store.CronTask.Delete(c.Request.Context(), req.ID, workerID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			apiError(c, http.StatusNotFound, "cron_task 不存在")
			return
		}
		apiError(c, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.reloadWorkersState(c.Request.Context()); err != nil {
		apiError(c, http.StatusInternalServerError, err.Error())
		return
	}
	apiSuccess(c, gin.H{"ok": true})
}

func (s *Server) requireWorker(c *gin.Context) (string, bool) {
	workerID := strings.TrimSpace(c.Param("id"))
	if workerID == "" {
		apiError(c, http.StatusBadRequest, "id 不能为空")
		return "", false
	}
	if _, err := s.store.Worker.GetByID(c.Request.Context(), workerID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			apiError(c, http.StatusNotFound, "函数不存在")
			return "", false
		}
		apiError(c, http.StatusInternalServerError, err.Error())
		return "", false
	}
	return workerID, true
}

func (s *Server) validateCronExpression(expr string) error {
	if strings.TrimSpace(expr) == "" {
		return fmt.Errorf("cron 不能为空")
	}
	return cron.ValidateExpression(expr)
}

func flattenFiles(fileMap map[string][]*multipart.FileHeader) []*multipart.FileHeader {
	result := make([]*multipart.FileHeader, 0)
	for _, files := range fileMap {
		result = append(result, files...)
	}
	return result
}

func renameWorkerFile(functionDir string, filename string, newFilename string) error {
	return workerpkg.RenameCodeFile(functionDir, filename, newFilename)
}
