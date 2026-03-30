package config

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Port              string
	AIPort            string
	AIServiceURL      string
	RedisEnabled      bool
	RedisAddr         string
	RedisPassword     string
	RedisDB           int
	OHLCCacheTTL      time.Duration
	SymbolsCacheTTL   time.Duration
	AIProvidersTTL    time.Duration
	SessionCacheTTL   time.Duration
	BillingCacheTTL   time.Duration
	AIRateWindow      time.Duration
	AIUserRateLimit   int
	AIIPRateLimit     int
	DatabaseURL       string
	CORSOrigins       []string
	AdminEmails       []string
	JWTSecret         string
	DefaultAIProvider string
	OpenAIBaseURL     string
	OpenAIAPIKey      string
	OpenAIModel       string
	DeepSeekBaseURL   string
	DeepSeekAPIKey    string
	DeepSeekModel     string
	MockPayments      bool
	DemoEmail         string
	DemoPassword      string
	TrackedSymbols    []string
	EastmoneyTimeout  time.Duration
	NewsTimeout       time.Duration
	EmbeddedPostgres  bool
	EmbeddedDataPath  string
	EmbeddedRunPath   string
}

func Load() Config {
	tmpDir := os.TempDir()

	return Config{
		Port:              env("PORT", "8080"),
		AIPort:            env("AI_PORT", "8081"),
		AIServiceURL:      strings.TrimRight(env("AI_SERVICE_URL", "http://localhost:8081"), "/"),
		RedisEnabled:      envBool("REDIS_ENABLED", true),
		RedisAddr:         env("REDIS_ADDR", "127.0.0.1:6379"),
		RedisPassword:     env("REDIS_PASSWORD", ""),
		RedisDB:           envInt("REDIS_DB", 0),
		OHLCCacheTTL:      2 * time.Minute,
		SymbolsCacheTTL:   5 * time.Minute,
		AIProvidersTTL:    5 * time.Minute,
		SessionCacheTTL:   time.Minute,
		BillingCacheTTL:   time.Minute,
		AIRateWindow:      time.Minute,
		AIUserRateLimit:   envInt("AI_USER_RATE_LIMIT", 20),
		AIIPRateLimit:     envInt("AI_IP_RATE_LIMIT", 60),
		DatabaseURL:       env("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/market_copilot?sslmode=disable"),
		CORSOrigins:       splitCSV(env("CORS_ORIGINS", "http://localhost:5173,http://127.0.0.1:5173")),
		AdminEmails:       splitCSV(env("ADMIN_EMAILS", "demo@example.com")),
		JWTSecret:         env("JWT_SECRET", "change-me-before-production"),
		DefaultAIProvider: env("DEFAULT_AI_PROVIDER", "deepseek"),
		OpenAIBaseURL:     strings.TrimRight(env("OPENAI_BASE_URL", env("OPENAI_PROVIDER_BASE_URL", "https://api.openai.com/v1")), "/"),
		OpenAIAPIKey:      env("OPENAI_API_KEY", env("OPENAI_PROVIDER_API_KEY", "")),
		OpenAIModel:       env("OPENAI_MODEL", env("OPENAI_PROVIDER_MODEL", "gpt-4.1-mini")),
		DeepSeekBaseURL:   strings.TrimRight(env("DEEPSEEK_BASE_URL", "https://api.deepseek.com/v1"), "/"),
		DeepSeekAPIKey:    env("DEEPSEEK_API_KEY", env("OPENAI_API_KEY", "")),
		DeepSeekModel:     env("DEEPSEEK_MODEL", "deepseek-chat"),
		MockPayments:      envBool("MOCK_PAYMENTS", true),
		DemoEmail:         env("DEMO_EMAIL", "demo@example.com"),
		DemoPassword:      env("DEMO_PASSWORD", "demo123456"),
		TrackedSymbols:    splitCSV(env("TRACKED_SYMBOLS", "600519,000001,300750")),
		EastmoneyTimeout:  12 * time.Second,
		NewsTimeout:       12 * time.Second,
		EmbeddedPostgres:  envBool("EMBEDDED_POSTGRES", true),
		EmbeddedDataPath:  env("EMBEDDED_POSTGRES_DATA_PATH", filepath.Join(tmpDir, "market-copilot-postgres-data")),
		EmbeddedRunPath:   env("EMBEDDED_POSTGRES_RUNTIME_PATH", filepath.Join(tmpDir, "market-copilot-postgres-runtime")),
	}
}

func env(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func splitCSV(raw string) []string {
	parts := strings.Split(raw, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}

func envBool(key string, fallback bool) bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	if value == "" {
		return fallback
	}
	return value == "1" || value == "true" || value == "yes"
}

func envInt(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}

	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}

	return parsed
}
