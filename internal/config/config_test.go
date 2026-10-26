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

func TestAppConfigKeysIncludesSupportedConfig(t *testing.T) {
	keys := AppConfigKeys()
	required := map[string]bool{
		"MCP_ENABLE":              false,
		"MCP_TOKEN":               false,
		WorkerLogRetentionDaysKey: false,
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

func TestConfigSetAndGetWorkerLogRetentionDays(t *testing.T) {
	var cfg Config
	if ok := cfg.SetAppConfigValue(WorkerLogRetentionDaysKey, "45"); !ok {
		t.Fatalf("设置日志保留天数失败")
	}
	value, ok := cfg.GetAppConfigValue(WorkerLogRetentionDaysKey)
	if !ok || value != "45" {
		t.Fatalf("读取日志保留天数失败: ok=%v value=%q", ok, value)
	}
	for _, invalid := range []string{"0", "-1", "invalid"} {
		if ok := cfg.SetAppConfigValue(WorkerLogRetentionDaysKey, invalid); ok {
			t.Fatalf("非法日志保留天数 %q 不应设置成功", invalid)
		}
	}
}

func TestConfigSyncLoadsDatabaseAppConfig(t *testing.T) {
	var cfg Config
	if err := cfg.Sync(context.Background(), testAppConfigDAO{
		items: map[string]string{
			"MCP_ENABLE":              "true",
			"MCP_TOKEN":               "db-token",
			WorkerLogRetentionDaysKey: "45",
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
	if cfg.AppConfig.WorkerLogRetentionDays != 45 {
		t.Fatalf("期望数据库覆盖后的日志保留天数为 45，实际为 %d", cfg.AppConfig.WorkerLogRetentionDays)
	}
}

func TestConfigSyncUsesDefaultWorkerLogRetentionDays(t *testing.T) {
	var cfg Config
	if err := cfg.Sync(context.Background(), nil); err != nil {
		t.Fatalf("同步默认配置失败: %v", err)
	}
	if cfg.AppConfig.WorkerLogRetentionDays != 30 {
		t.Fatalf("默认日志保留天数应为 30，实际为 %d", cfg.AppConfig.WorkerLogRetentionDays)
	}
}

func TestLoadUsesWorkerRunningTempDirAndRuntimeLibDir(t *testing.T) {
	t.Setenv("DATA_DIR", "/opt/callit")
	t.Setenv("WORKER_RUNNING_TEMP_DIR", "/opt/callit/tmp")
	t.Setenv("RUNTIME_LIB_DIR", "/opt/callit/runtime-lib")
	t.Setenv("ENABLE_CGROUP_V2", "true")
	t.Setenv("REDIS_ADDR", "redis:6379")
	t.Setenv("REDIS_PASSWORD", "pwd")
	t.Setenv("REDIS_DB", "2")

	cfg := Load()
	if cfg.DataDir != "/opt/callit" {
		t.Fatalf("期望 DataDir 被环境变量覆盖，实际为 %q", cfg.DataDir)
	}
	if cfg.WorkerRunningTempDir != "/opt/callit/tmp" {
		t.Fatalf("期望 WorkerRunningTempDir 被环境变量覆盖，实际为 %q", cfg.WorkerRunningTempDir)
	}
	if cfg.RuntimeLibDir != "/opt/callit/runtime-lib" {
		t.Fatalf("期望 RuntimeLibDir 被环境变量覆盖，实际为 %q", cfg.RuntimeLibDir)
	}
	if !cfg.EnableCgroupV2 {
		t.Fatalf("期望 EnableCgroupV2 被环境变量覆盖为 true")
	}
	if cfg.RedisAddr != "redis:6379" || cfg.RedisPassword != "pwd" || cfg.RedisDB != 2 {
		t.Fatalf("期望 Redis 配置被环境变量覆盖，got=%+v", cfg)
	}
}

func TestLoadUsesDefaultRedisAddr(t *testing.T) {
	cfg := Load()
	if cfg.RedisAddr != "redis:6379" {
		t.Fatalf("期望默认 RedisAddr 为 redis:6379，实际为 %q", cfg.RedisAddr)
	}
}

func TestLoadUsesWorkerMaxDirDepth(t *testing.T) {
	t.Setenv("WORKER_MAX_DIR_DEPTH", "5")
	if got := Load().WorkerMaxDirDepth; got != 5 {
		t.Fatalf("期望 WorkerMaxDirDepth 为 5，实际为 %d", got)
	}
}

func TestLoadUsesDefaultWorkerMaxDirDepthForInvalidValue(t *testing.T) {
	for _, value := range []string{"", "invalid", "0", "-1"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("WORKER_MAX_DIR_DEPTH", value)
			if got := Load().WorkerMaxDirDepth; got != 3 {
				t.Fatalf("非法值 %q 应回退为 3，实际为 %d", value, got)
			}
		})
	}
}
