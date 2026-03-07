package admin

import (
	"context"
	"net/http"
	"os"
	"strings"
	"time"

	"callit/internal/config"

	"github.com/gin-gonic/gin"
)

type adminConfigItem struct {
	Key    string  `json:"key"`
	Value  string  `json:"value"`
	Source string  `json:"source"`
	DB     *string `json:"db,omitempty"`
}

type adminUpsertConfigPayload struct {
	AppConfig map[string]*string `json:"app_config"`
}

func (s *Server) AdminGetConfigHandler(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()

		dbItems, err := s.store.AppConfig.GetConfigs(ctx)
		if err != nil {
			apiError(c, http.StatusInternalServerError, "读取配置失败")
			return
		}

		keys := config.AppConfigKeys()
		items := make([]adminConfigItem, 0, len(keys))

		s.configMu.RLock()
		defer s.configMu.RUnlock()
		for _, key := range keys {
			val, _ := cfg.GetAppConfigValue(key)
			item := adminConfigItem{
				Key:    key,
				Value:  val,
				Source: "default",
			}
			if dbv, ok := dbItems[key]; ok {
				item.Source = "db"
				item.DB = &dbv
			} else if os.Getenv(key) != "" {
				item.Source = "env"
			}
			items = append(items, item)
		}

		apiSuccess(c, items)
	}
}

func (s *Server) AdminUpsertConfigHandler(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req adminUpsertConfigPayload
		if err := c.ShouldBindJSON(&req); err != nil {
			apiError(c, http.StatusBadRequest, "参数错误")
			return
		}
		if len(req.AppConfig) == 0 {
			apiError(c, http.StatusBadRequest, "缺少配置项")
			return
		}

		updates := make(map[string]string, len(req.AppConfig))
		for key, value := range req.AppConfig {
			if value == nil {
				continue
			}
			normalizedKey := strings.ToUpper(strings.TrimSpace(key))
			if normalizedKey == "" {
				apiError(c, http.StatusBadRequest, "配置 key 不能为空")
				return
			}
			if !config.IsAppConfigKey(normalizedKey) {
				apiError(c, http.StatusBadRequest, "配置项不在白名单中")
				return
			}
			updates[normalizedKey] = *value
		}
		if len(updates) == 0 {
			apiError(c, http.StatusBadRequest, "缺少可更新的配置项")
			return
		}

		s.configMu.RLock()
		nextCfg := *cfg
		s.configMu.RUnlock()
		for key, value := range updates {
			if ok := nextCfg.SetAppConfigValue(key, value); !ok {
				apiError(c, http.StatusBadRequest, "配置值格式错误")
				return
			}
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
		defer cancel()

		s.configMu.Lock()
		defer s.configMu.Unlock()
		for key, value := range updates {
			if err := s.store.AppConfig.SetConfig(ctx, cfg, key, value); err != nil {
				apiError(c, http.StatusInternalServerError, "保存配置失败")
				return
			}
		}

		s.chatHandler.ReloadAIConfig(cfg.AppConfig)
		apiSuccess(c, gin.H{"success": true})
	}
}
