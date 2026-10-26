package admin

import (
	"context"
	"net/http"
	"time"

	"callit/internal/admin/storage"

	"github.com/gin-gonic/gin"
)

func (server *Server) storageDBContext(request *gin.Context) (context.Context, context.CancelFunc, bool) {
	if server.storageDBSvc == nil {
		apiError(request, http.StatusServiceUnavailable, "Worker 数据库服务不可用")
		return nil, nil, false
	}
	ctx, cancel := context.WithTimeout(request.Request.Context(), 5*time.Second)
	return ctx, cancel, true
}

func (server *Server) listStorageDBTables(request *gin.Context) {
	ctx, cancel, available := server.storageDBContext(request)
	if !available {
		return
	}
	defer cancel()
	tables, err := server.storageDBSvc.Tables(ctx)
	if err != nil {
		apiError(request, http.StatusInternalServerError, err.Error())
		return
	}
	apiSuccess(request, tables)
}

func (server *Server) queryStorageDB(request *gin.Context) {
	ctx, cancel, available := server.storageDBContext(request)
	if !available {
		return
	}
	defer cancel()
	var query storage.DBQuery
	if err := request.ShouldBindJSON(&query); err != nil {
		apiError(request, http.StatusBadRequest, "请求体格式错误")
		return
	}
	result, err := server.storageDBSvc.Query(ctx, query)
	if err != nil {
		apiError(request, http.StatusBadRequest, err.Error())
		return
	}
	apiSuccess(request, result)
}

func (server *Server) createStorageDBRow(request *gin.Context) {
	server.writeStorageDBRow(request, true)
}

func (server *Server) updateStorageDBRow(request *gin.Context) {
	server.writeStorageDBRow(request, false)
}

func (server *Server) writeStorageDBRow(request *gin.Context, create bool) {
	ctx, cancel, available := server.storageDBContext(request)
	if !available {
		return
	}
	defer cancel()
	var body storage.DBWriteRequest
	if err := request.ShouldBindJSON(&body); err != nil {
		apiError(request, http.StatusBadRequest, "请求体格式错误")
		return
	}
	if err := server.storageDBSvc.Write(ctx, body, create); err != nil {
		apiError(request, http.StatusBadRequest, err.Error())
		return
	}
	apiSuccess(request, gin.H{"ok": true})
}

func (server *Server) deleteStorageDBRows(request *gin.Context) {
	ctx, cancel, available := server.storageDBContext(request)
	if !available {
		return
	}
	defer cancel()
	var body storage.DBDeleteRequest
	if err := request.ShouldBindJSON(&body); err != nil {
		apiError(request, http.StatusBadRequest, "请求体格式错误")
		return
	}
	if err := server.storageDBSvc.Delete(ctx, body); err != nil {
		apiError(request, http.StatusBadRequest, err.Error())
		return
	}
	apiSuccess(request, gin.H{"ok": true})
}
