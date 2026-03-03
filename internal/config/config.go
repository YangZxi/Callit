package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	defaultRouterPort = 3100
	defaultAdminPort  = 3101
	defaultDataDir    = "data"
	defaultAIBaseURL  = "https://api.openai.com/v1"
	defaultAIModel    = "gpt-4o-mini"
	defaultAITimeout  = 120000
	defaultAIMaxToken = 16000
)

// AIConfig 表示 AI 客户端配置。
type AIConfig struct {
	BaseURL          string
	APIKey           string
	Model            string
	MaxContextTokens int
	TimeoutMS        int
}

// Config 表示应用运行配置。
type Config struct {
	RouterPort int
	AdminPort  int
	AdminToken string
	DataDir    string
	DBPath     string
	AI         AIConfig
}

// Load 从环境变量加载配置。
func Load() (Config, error) {
	cfg := Config{}

	routerPort, err := readIntEnv("ROUTER_PORT", defaultRouterPort)
	if err != nil {
		return Config{}, err
	}
	adminPort, err := readIntEnv("ADMIN_PORT", defaultAdminPort)
	if err != nil {
		return Config{}, err
	}

	dataDir := os.Getenv("DATA_DIR")
	if dataDir == "" {
		dataDir = defaultDataDir
	}

	adminToken := os.Getenv("ADMIN_TOKEN")
	if adminToken == "" {
		return Config{}, fmt.Errorf("环境变量 ADMIN_TOKEN 不能为空")
	}

	cfg.RouterPort = routerPort
	cfg.AdminPort = adminPort
	cfg.AdminToken = adminToken
	cfg.DataDir = dataDir
	cfg.DBPath = filepath.Join(dataDir, "app.db")

	maxTokens, err := readIntEnv("AI_MAX_CONTEXT_TOKENS", defaultAIMaxToken)
	if err != nil {
		return Config{}, err
	}
	timeoutMS, err := readIntEnv("AI_TIMEOUT_MS", defaultAITimeout)
	if err != nil {
		return Config{}, err
	}

	aiBaseURL := strings.TrimSpace(os.Getenv("AI_BASE_URL"))
	if aiBaseURL == "" {
		aiBaseURL = defaultAIBaseURL
	}
	aiModel := strings.TrimSpace(os.Getenv("AI_MODEL"))
	if aiModel == "" {
		aiModel = defaultAIModel
	}
	cfg.AI = AIConfig{
		BaseURL:          strings.TrimRight(aiBaseURL, "/"),
		APIKey:           strings.TrimSpace(os.Getenv("AI_API_KEY")),
		Model:            aiModel,
		MaxContextTokens: maxTokens,
		TimeoutMS:        timeoutMS,
	}
	return cfg, nil
}

func readIntEnv(key string, def int) (int, error) {
	val := os.Getenv(key)
	if val == "" {
		return def, nil
	}
	n, err := strconv.Atoi(val)
	if err != nil {
		return 0, fmt.Errorf("环境变量 %s 不是合法整数: %w", key, err)
	}
	return n, nil
}
