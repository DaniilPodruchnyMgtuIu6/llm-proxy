package config

import (
	"fmt"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	Addr              string
	DatabasePath      string
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
		Addr:              envOr("ADDR", ":8080"),
		DatabasePath:      envOr("DATABASE_PATH", "data/llm-proxy.db"),
		OpenAPIPath:       envOr("OPENAPI_PATH", "api/openapi.yaml"),
		GeminiAPIKey:      os.Getenv("GEMINI_API_KEY"),
		GeminiBaseURL:     envOr("GEMINI_BASE_URL", "https://generativelanguage.googleapis.com/v1beta/openai"),
		GroqAPIKey:        os.Getenv("GROQ_API_KEY"),
		GroqBaseURL:       envOr("GROQ_BASE_URL", "https://api.groq.com/openai/v1"),
		OpenRouterAPIKey:  os.Getenv("OPENROUTER_API_KEY"),
		OpenRouterBaseURL: envOr("OPENROUTER_BASE_URL", "https://openrouter.ai/api/v1"),
		OpenRouterSiteURL: envOr("OPENROUTER_SITE_URL", "http://localhost:8080"),
		OpenRouterTitle:   envOr("OPENROUTER_SITE_TITLE", "llm-proxy"),
	}

	if cfg.GeminiAPIKey == "" && cfg.GroqAPIKey == "" && cfg.OpenRouterAPIKey == "" {
		return Config{}, fmt.Errorf("at least one provider API key is required (GEMINI_API_KEY, GROQ_API_KEY, OPENROUTER_API_KEY)")
	}

	if port := os.Getenv("PORT"); port != "" {
		if _, err := strconv.Atoi(port); err == nil {
			cfg.Addr = ":" + port
		}
	}

	return cfg, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
