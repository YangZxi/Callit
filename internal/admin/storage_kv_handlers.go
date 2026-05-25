package admin

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

type deleteStorageKVKeyRequest struct {
	Key string `json:"key"`
}

func (s *Server) getStorageKVMetrics(c *gin.Context) {
	if s.storageKvSvc == nil {
		apiError(c, http.StatusInternalServerError, "kv service unavailable")
		return
	}
	metrics, err := s.storageKvSvc.Metrics(c.Request.Context())
	if err != nil {
		apiError(c, http.StatusInternalServerError, err.Error())
		return
	}
	apiSuccess(c, metrics)
}

func (s *Server) listStorageKVKeys(c *gin.Context) {
	if s.storageKvSvc == nil {
		apiError(c, http.StatusInternalServerError, "kv service unavailable")
		return
	}
	items, err := s.storageKvSvc.ListKeys(c.Request.Context(), c.Query("key"))
	if err != nil {
		apiError(c, http.StatusInternalServerError, err.Error())
		return
	}
	apiSuccess(c, items)
}

func (s *Server) getStorageKVKeyDetail(c *gin.Context) {
	if s.storageKvSvc == nil {
		apiError(c, http.StatusInternalServerError, "kv service unavailable")
		return
	}
	key := strings.TrimSpace(c.Query("key"))
	if key == "" {
		apiError(c, http.StatusBadRequest, "key 不能为空")
		return
	}
	detail, err := s.storageKvSvc.Detail(c.Request.Context(), key)
	if err != nil {
		apiError(c, http.StatusBadRequest, err.Error())
		return
	}
	apiSuccess(c, detail)
}

func (s *Server) deleteStorageKVKey(c *gin.Context) {
	if s.storageKvSvc == nil {
		apiError(c, http.StatusInternalServerError, "kv service unavailable")
		return
	}
	var req deleteStorageKVKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiError(c, http.StatusBadRequest, "请求体格式错误")
		return
	}
	if strings.TrimSpace(req.Key) == "" {
		apiError(c, http.StatusBadRequest, "key 不能为空")
		return
	}
	if err := s.storageKvSvc.Delete(c.Request.Context(), req.Key); err != nil {
		apiError(c, http.StatusBadRequest, err.Error())
		return
	}
	apiSuccess(c, gin.H{"ok": true})
}
