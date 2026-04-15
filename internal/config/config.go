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

// AppConfig 中的值会被数据库覆盖
// 如果你的配置不需要被数据库接管，请不要放在这里
type AppConfig struct {
	// MCP
	MCP_Enable bool   `config:"MCP_ENABLE"`
	MCP_Token  string `config:"MCP_TOKEN"`
}

type Config struct {
	LogLevel string

	ServerPort           int
	MagicServerPort      int
	AdminPrefix          string
	AdminToken           string
	DataDir              string
	DatabasePath         string
	WorkersDir           string
	WorkerRunningTempDir string
	RuntimeLibDir        string
	MaxFileSize          int64

	// Runtime
	EnableCgroupV2 bool
	RedisAddr      string
	RedisPassword  string
	RedisDB        int

	AppConfig AppConfig
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
		MagicServerPort:      31001,
		AdminToken:           getEnv("ADMIN_TOKEN", ""),
		AdminPrefix:          getEnv("ADMIN_PREFIX", "admin"),
		DataDir:              getEnv("DATA_DIR", "./data"),
		DatabasePath:         getEnv("DATABASE_PATH", "./data/app.db"),
		WorkersDir:           getEnv("WORKERS_DIR", "./data/workers"),
		WorkerRunningTempDir: getEnv("WORKER_RUNNING_TEMP_DIR", "./data/tmp"),
		RuntimeLibDir:        getEnv("RUNTIME_LIB_DIR", "./data/.lib"),

		MaxFileSize: (1 << 20) * 100, // 100MB

		LogLevel:       getEnv("LOG_LEVEL", "info"),
		EnableCgroupV2: getBool("ENABLE_CGROUP_V2", false),
		RedisAddr:      getEnv("REDIS_ADDR", "redis:6379"),
		RedisPassword:  getEnv("REDIS_PASSWORD", ""),
		RedisDB:        getInt("REDIS_DB", 0),
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
		MCP_Enable: getBool("MCP_ENABLE", false),
		MCP_Token:  getEnv("MCP_TOKEN", ""),
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
