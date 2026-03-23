package config

import (
	"context"
	"testing"
)

type testAppConfigDAO struct {
	items map[string]string
}

func (dao testAppConfigDAO) GetConfigs(context.Context) (map[string]string, error) {
	return dao.items, nil
}

func TestAppConfigKeysIncludesMCPConfig(t *testing.T) {
	keys := AppConfigKeys()
	required := map[string]bool{
		"MCP_ENABLE": false,
		"MCP_TOKEN":  false,
	}
	for _, key := range keys {
		if _, ok := required[key]; ok {
			required[key] = true
		}
	}
	for key, found := range required {
		if !found {
			t.Fatalf("AppConfigKeys 未包含 %s", key)
		}
	}
}

func TestConfigSetAndGetMCPConfigValue(t *testing.T) {
	var cfg Config

	if ok := cfg.SetAppConfigValue("MCP_ENABLE", "true"); !ok {
		t.Fatalf("设置 MCP_ENABLE 失败")
	}
	if ok := cfg.SetAppConfigValue("MCP_TOKEN", "secret-token"); !ok {
		t.Fatalf("设置 MCP_TOKEN 失败")
	}

	enableValue, ok := cfg.GetAppConfigValue("MCP_ENABLE")
	if !ok || enableValue != "true" {
		t.Fatalf("读取 MCP_ENABLE 失败: ok=%v value=%q", ok, enableValue)
	}
	tokenValue, ok := cfg.GetAppConfigValue("MCP_TOKEN")
	if !ok || tokenValue != "secret-token" {
		t.Fatalf("读取 MCP_TOKEN 失败: ok=%v value=%q", ok, tokenValue)
	}
}

func TestConfigSyncLoadsDefaultAndDBMCPConfig(t *testing.T) {
	var cfg Config
	if err := cfg.Sync(context.Background(), testAppConfigDAO{
		items: map[string]string{
			"MCP_ENABLE": "true",
			"MCP_TOKEN":  "db-token",
		},
	}); err != nil {
		t.Fatalf("同步配置失败: %v", err)
	}

	if !cfg.AppConfig.MCP_Enable {
		t.Fatalf("期望数据库覆盖后的 MCP_ENABLE 为 true")
	}
	if cfg.AppConfig.MCP_Token != "db-token" {
		t.Fatalf("期望数据库覆盖后的 MCP_TOKEN 为 db-token，实际为 %q", cfg.AppConfig.MCP_Token)
	}
}
