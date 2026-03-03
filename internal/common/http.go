package common

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const RequestIDKey = "request_id"

// RequestIDMiddleware 注入 request_id 到上下文与响应头。
func RequestIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		reqID := uuid.NewString()
		c.Set(RequestIDKey, reqID)
		c.Header("X-Request-ID", reqID)
		c.Next()
	}
}

// GetRequestID 获取当前请求 ID。
func GetRequestID(c *gin.Context) string {
	val, ok := c.Get(RequestIDKey)
	if !ok {
		return ""
	}
	if s, ok := val.(string); ok {
		return s
	}
	return ""
}

// ErrorResponse 统一错误输出。
func ErrorResponse(c *gin.Context, code int, message string) {
	c.JSON(code, gin.H{
		"error":      message,
		"request_id": GetRequestID(c),
	})
}

// UnauthorizedResponse 统一鉴权失败输出。
func UnauthorizedResponse(c *gin.Context) {
	ErrorResponse(c, http.StatusUnauthorized, "unauthorized")
}
