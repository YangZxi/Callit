package config

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/google/uuid"
)

type AppConfig struct {
	// AI
	AI_BaseURL          string `config:"AI_BASE_URL"`
	AI_APIKey           string `config:"AI_API_KEY"`
	AI_Model            string `config:"AI_MODEL"`
	AI_MaxContextTokens int    `config:"AI_MAX_CONTEXT_TOKENS"`
	AI_TimeoutMS        int    `config:"AI_TIMEOUT_MS"`
}

type Config struct {
	ServerPort           int
	AdminPrefix          string
	DataDir              string
	DatabasePath         string
	WorkersDir           string
	WorkerRunningTempDir string
	ChatSessionsDir      string
	RuntimeLibDir        string
	MaxFileSize          int64
	AdminToken           string
	LogLevel             string
	AppConfig            AppConfig
}

type AppConfigDao interface {
	GetConfigs(ctx context.Context) (map[string]string, error)
}

var (
	appConfigKeyOnce  sync.Once
	appConfigKeyIndex map[string][]int
)

func getAppConfigKeyIndex() map[string][]int {
	appConfigKeyOnce.Do(func() {
		appConfigKeyIndex = make(map[string][]int)
		t := reflect.TypeOf(AppConfig{})
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			key := strings.ToUpper(strings.TrimSpace(f.Tag.Get("config")))
			if key == "" {
				continue
			}
			appConfigKeyIndex[key] = f.Index
		}
	})
	return appConfigKeyIndex
}

// AppConfigKeys 返回所有 AppConfig 白名单 key（来自结构体 tag），并按字母序排序。
func AppConfigKeys() []string {
	index := getAppConfigKeyIndex()
	keys := make([]string, 0, len(index))
	for k := range index {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func Load() Config {
	cfg := Config{
		ServerPort:           getInt("SERVER_PORT", 3100),
		AdminToken:           getEnv("ADMIN_TOKEN", ""),
		AdminPrefix:          getEnv("ADMIN_PREFIX", "admin"),
		DataDir:              getEnv("DATA_DIR", "./data"),
		DatabasePath:         getEnv("DATABASE_PATH", "./data/app.db"),
		WorkersDir:           getEnv("WORKERS_DIR", "./data/workers"),
		WorkerRunningTempDir: getEnv("WORKER_RUNNING_TEMP_DIR", "./data/temp"),
		ChatSessionsDir:      getEnv("CHAT_SESSIONS_DIR", "./data/chat-sessions"),
		RuntimeLibDir:        getEnv("RUNTIME_LIB_DIR", "./data/.lib"),

		MaxFileSize: (1 << 20) * 100, // 100MB

		LogLevel: getEnv("LOG_LEVEL", "info"),
	}
	nomralizeConfig(&cfg)
	return cfg
}

func nomralizeConfig(cfg *Config) {
	if cfg.AdminToken == "" {
		cfg.AdminToken = uuid.New().String()
		fmt.Printf("系统已随机生成 AdminToken，如需固定 Token，请设置环境变量 ADMIN_TOKEN\n")
	}

	if cfg.AdminPrefix == "" {
		cfg.AdminPrefix = "/admin"
	} else if !strings.HasPrefix(cfg.AdminPrefix, "/") {
		cfg.AdminPrefix = "/" + cfg.AdminPrefix
	}
	cfg.AdminPrefix = strings.TrimSuffix(cfg.AdminPrefix, "/")
}

func IsAppConfigKey(key string) bool {
	key = strings.ToUpper(strings.TrimSpace(key))
	_, ok := getAppConfigKeyIndex()[key]
	return ok
}

// SetAppConfigValue 仅对 AppConfig 白名单 key 生效；非白名单 key 会被忽略。
func (cfg *Config) SetAppConfigValue(key, value string) bool {
	key = strings.ToUpper(strings.TrimSpace(key))
	index, ok := getAppConfigKeyIndex()[key]
	if !ok {
		return false
	}
	v := reflect.ValueOf(&cfg.AppConfig).Elem()
	f := v.FieldByIndex(index)
	if !f.IsValid() || !f.CanSet() {
		return false
	}

	switch f.Kind() {
	case reflect.String:
		f.SetString(value)
		return true
	case reflect.Bool:
		b, err := strconv.ParseBool(value)
		if err != nil {
			return false
		}
		f.SetBool(b)
		return true
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		// 若未来 AppConfig 有时长字段，可在此扩展（例如识别 time.Duration）
		i, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return false
		}
		f.SetInt(i)
		return true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		u, err := strconv.ParseUint(value, 10, 64)
		if err != nil {
			return false
		}
		f.SetUint(u)
		return true
	case reflect.Float32, reflect.Float64:
		fl, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return false
		}
		f.SetFloat(fl)
		return true
	default:
		return false
	}
}

// GetAppConfigValue 获取 AppConfig 白名单配置的当前值（用于展示/回显）。
func (cfg *Config) GetAppConfigValue(key string) (string, bool) {
	key = strings.ToUpper(strings.TrimSpace(key))
	index, ok := getAppConfigKeyIndex()[key]
	if !ok {
		return "", false
	}
	v := reflect.ValueOf(cfg.AppConfig)
	f := v.FieldByIndex(index)
	if !f.IsValid() {
		return "", false
	}
	switch f.Kind() {
	case reflect.String:
		return f.String(), true
	case reflect.Bool:
		return strconv.FormatBool(f.Bool()), true
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(f.Int(), 10), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return strconv.FormatUint(f.Uint(), 10), true
	case reflect.Float32, reflect.Float64:
		return strconv.FormatFloat(f.Float(), 'f', -1, 64), true
	default:
		return "", false
	}
}

// Sync 用于在 Load() 之后同步 AppConfig。
// 优先级：数据库 > env > 硬编码。
func (cfg *Config) Sync(ctx context.Context, dao AppConfigDao) error {
	cfg.AppConfig = AppConfig{
		AI_BaseURL:          getEnv("AI_BASE_URL", "https://api.openai.com/v1"),
		AI_APIKey:           getEnv("AI_API_KEY", ""),
		AI_Model:            getEnv("AI_MODEL", "gpt-5"),
		AI_MaxContextTokens: getInt("AI_MAX_CONTEXT_TOKENS", 16000),
		AI_TimeoutMS:        getInt("AI_TIMEOUT_MS", 60000),
	}
	if dao == nil {
		return nil
	}
	items, err := dao.GetConfigs(ctx)
	if err != nil {
		return err
	}
	for k, v := range items {
		if v == "" {
			continue
		}
		cfg.SetAppConfigValue(k, v)
	}
	return nil
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return def
}

func getBool(key string, def bool) bool {
	if v := os.Getenv(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return def
}
