package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	Addr              string
	LogLevel          string
	ProxyAPIKey       string
	UpstreamTimeout   time.Duration
	MaxFallbackAttempts int
	ModelsCacheTTL    time.Duration
	CircuitErrors     int64
	CircuitCooldown   time.Duration
	DBHost            string
	DBPort            string
	DBUser            string
	DBPassword        string
	DBName            string
	DBSSLMode         string
	DatabaseURL       string // assembled from DB_* for store/pgx
	OpenAPIPath       string
	GeminiAPIKey      string
	GeminiBaseURL     string
	GroqAPIKey        string
	GroqBaseURL       string
	OpenRouterAPIKey  string
	OpenRouterBaseURL string
	OpenRouterSiteURL string
	OpenRouterTitle   string
}

func Load() (Config, error) {
	_ = godotenv.Load()

	cfg := Config{
		Addr:                envOr("ADDR", ":8080"),
		LogLevel:            envOr("LOG_LEVEL", "info"),
		ProxyAPIKey:         os.Getenv("PROXY_API_KEY"),
		UpstreamTimeout:     envDuration("UPSTREAM_TIMEOUT", 60*time.Second),
		MaxFallbackAttempts: envInt("MAX_FALLBACK_ATTEMPTS", 3),
		ModelsCacheTTL:      envDuration("MODELS_CACHE_TTL", 45*time.Second),
		CircuitErrors:       int64(envInt("CIRCUIT_BREAKER_ERRORS", 5)),
		CircuitCooldown:     envDuration("CIRCUIT_BREAKER_COOLDOWN", 5*time.Minute),
		DBHost:              envOr("DB_HOST", "localhost"),
		DBPort:              envOr("DB_PORT", "5432"),
		DBUser:              envOr("DB_USER", "llmproxy"),
		DBPassword:          envOr("DB_PASSWORD", "llmproxy"),
		DBName:              envOr("DB_NAME", "llmproxy"),
		DBSSLMode:           envOr("DB_SSLMODE", "disable"),
		OpenAPIPath:         envOr("OPENAPI_PATH", "api/openapi.yaml"),
		GeminiAPIKey:        os.Getenv("GEMINI_API_KEY"),
		GeminiBaseURL:       envOr("GEMINI_BASE_URL", "https://generativelanguage.googleapis.com/v1beta/openai"),
		GroqAPIKey:          os.Getenv("GROQ_API_KEY"),
		GroqBaseURL:         envOr("GROQ_BASE_URL", "https://api.groq.com/openai/v1"),
		OpenRouterAPIKey:    os.Getenv("OPENROUTER_API_KEY"),
		OpenRouterBaseURL:   envOr("OPENROUTER_BASE_URL", "https://openrouter.ai/api/v1"),
		OpenRouterSiteURL:   envOr("OPENROUTER_SITE_URL", "http://localhost:8080"),
		OpenRouterTitle:     envOr("OPENROUTER_SITE_TITLE", "llm-proxy"),
	}

	if cfg.MaxFallbackAttempts < 1 {
		cfg.MaxFallbackAttempts = 1
	}
	if cfg.UpstreamTimeout < time.Second {
		cfg.UpstreamTimeout = time.Second
	}

	if cfg.GeminiAPIKey == "" && cfg.GroqAPIKey == "" && cfg.OpenRouterAPIKey == "" {
		return Config{}, fmt.Errorf("at least one provider API key is required (GEMINI_API_KEY, GROQ_API_KEY, OPENROUTER_API_KEY)")
	}
	if cfg.DBHost == "" || cfg.DBPort == "" || cfg.DBUser == "" || cfg.DBName == "" {
		return Config{}, fmt.Errorf("DB_HOST, DB_PORT, DB_USER, DB_NAME are required")
	}

	cfg.DatabaseURL = buildPostgresURL(cfg)

	if port := os.Getenv("PORT"); port != "" {
		if _, err := strconv.Atoi(port); err == nil {
			cfg.Addr = ":" + port
		}
	}

	return cfg, nil
}

func buildPostgresURL(cfg Config) string {
	u := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(cfg.DBUser, cfg.DBPassword),
		Host:   fmt.Sprintf("%s:%s", cfg.DBHost, cfg.DBPort),
		Path:   "/" + cfg.DBName,
	}
	q := url.Values{}
	q.Set("sslmode", cfg.DBSSLMode)
	u.RawQuery = q.Encode()
	return u.String()
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

func envDuration(key string, fallback time.Duration) time.Duration {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	if d, err := time.ParseDuration(v); err == nil {
		return d
	}
	if n, err := strconv.Atoi(v); err == nil && n > 0 {
		return time.Duration(n) * time.Second
	}
	return fallback
}
