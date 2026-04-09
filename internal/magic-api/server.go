package magicapi

import (
	"context"
	"net/http"
	"strings"
	"time"

	magicDB "callit/internal/magic-api/db"
	magicKV "callit/internal/magic-api/kv"

	"github.com/gin-gonic/gin"
)

type Options struct {
	KVService *magicKV.Service
	DBService *magicDB.Service
}

type Server struct {
	kvService *magicKV.Service
	dbService *magicDB.Service
}

type kvCommandRequest struct {
	Key       string `json:"key"`
	Value     string `json:"value"`
	Namespace string `json:"namespace"`
	Seconds   *int64 `json:"seconds"`
	Step      *int64 `json:"step"`
}

type dbExecRequest struct {
	SQL  string `json:"sql"`
	Args []any  `json:"args"`
}

func NewHandler(opts Options) http.Handler {
	s := &Server{
		kvService: opts.KVService,
		dbService: opts.DBService,
	}
	engine := gin.New()
	engine.Use(gin.Recovery())
	engine.POST("/kv/:action", s.handleKV)
	engine.POST("/db/exec", s.handleDBExec)
	return engine
}

func (s *Server) handleKV(c *gin.Context) {
	workerID := strings.TrimSpace(c.GetHeader("X-Callit-Worker-Id"))
	requestID := strings.TrimSpace(c.GetHeader("X-Callit-Request-Id"))
	if workerID == "" || requestID == "" {
		magicKV.WriteJSONError(c.Writer, http.StatusUnauthorized, "unauthorized")
		return
	}
	if s.kvService == nil {
		magicKV.WriteJSONError(c.Writer, http.StatusInternalServerError, "kv service unavailable")
		return
	}

	var req kvCommandRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		magicKV.WriteJSONError(c.Writer, http.StatusBadRequest, "invalid request body")
		return
	}

	action := strings.TrimSpace(c.Param("action"))
	if strings.TrimSpace(req.Namespace) == "" {
		magicKV.WriteJSONError(c.Writer, http.StatusBadRequest, "namespace is required")
		return
	}
	switch action {
	case "set":
		if req.Seconds == nil {
			magicKV.WriteJSONError(c.Writer, http.StatusBadRequest, "seconds is required")
			return
		}
		if err := s.kvService.Set(c.Request.Context(), req.Namespace, req.Key, req.Value, time.Duration(*req.Seconds)*time.Second); err != nil {
			magicKV.WriteJSONError(c.Writer, http.StatusBadRequest, err.Error())
			return
		}
		c.Status(http.StatusOK)
	case "get":
		value, ok, err := s.kvService.Get(c.Request.Context(), req.Namespace, req.Key)
		if err != nil {
			magicKV.WriteJSONError(c.Writer, http.StatusBadRequest, err.Error())
			return
		}
		c.JSON(http.StatusOK, map[string]any{"value": func() any {
			if !ok {
				return nil
			}
			return value
		}()})
	case "delete":
		if err := s.kvService.Delete(c.Request.Context(), req.Namespace, req.Key); err != nil {
			magicKV.WriteJSONError(c.Writer, http.StatusBadRequest, err.Error())
			return
		}
		c.Status(http.StatusOK)
	case "increment":
		step := int64(1)
		if req.Step != nil {
			step = *req.Step
		}
		value, err := s.kvService.Increment(c.Request.Context(), req.Namespace, req.Key, step)
		if err != nil {
			magicKV.WriteJSONError(c.Writer, http.StatusBadRequest, err.Error())
			return
		}
		c.JSON(http.StatusOK, map[string]int64{"int_value": value})
	case "expire":
		if req.Seconds == nil {
			magicKV.WriteJSONError(c.Writer, http.StatusBadRequest, "seconds is required")
			return
		}
		if err := s.kvService.Expire(c.Request.Context(), req.Namespace, req.Key, time.Duration(*req.Seconds)*time.Second); err != nil {
			magicKV.WriteJSONError(c.Writer, http.StatusBadRequest, err.Error())
			return
		}
		c.Status(http.StatusOK)
	case "ttl":
		value, err := s.kvService.TTL(c.Request.Context(), req.Namespace, req.Key)
		if err != nil {
			magicKV.WriteJSONError(c.Writer, http.StatusBadRequest, err.Error())
			return
		}
		c.JSON(http.StatusOK, map[string]int64{"seconds": value})
	case "has":
		value, err := s.kvService.Has(c.Request.Context(), req.Namespace, req.Key)
		if err != nil {
			magicKV.WriteJSONError(c.Writer, http.StatusBadRequest, err.Error())
			return
		}
		c.JSON(http.StatusOK, map[string]bool{"exists": value})
	default:
		magicKV.WriteJSONError(c.Writer, http.StatusNotFound, "unsupported kv action")
	}
}

func (s *Server) handleDBExec(c *gin.Context) {
	workerID := strings.TrimSpace(c.GetHeader("X-Callit-Worker-Id"))
	requestID := strings.TrimSpace(c.GetHeader("X-Callit-Request-Id"))
	if workerID == "" || requestID == "" {
		magicKV.WriteJSONError(c.Writer, http.StatusUnauthorized, "unauthorized")
		return
	}
	if s.dbService == nil {
		magicKV.WriteJSONError(c.Writer, http.StatusInternalServerError, "db service unavailable")
		return
	}

	var req dbExecRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		magicKV.WriteJSONError(c.Writer, http.StatusBadRequest, "invalid request body")
		return
	}
	if strings.TrimSpace(req.SQL) == "" {
		magicKV.WriteJSONError(c.Writer, http.StatusBadRequest, "sql is required")
		return
	}

	result, err := s.dbService.Exec(c.Request.Context(), req.SQL, req.Args)
	if err != nil {
		magicKV.WriteJSONError(c.Writer, http.StatusBadRequest, err.Error())
		return
	}
	c.JSON(http.StatusOK, result)
}

func Shutdown(ctx context.Context, server *http.Server) error {
	if server == nil {
		return nil
	}
	return server.Shutdown(ctx)
}
