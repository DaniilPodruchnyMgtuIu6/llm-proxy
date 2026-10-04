package main

import (
	"net/http"
	"os"

	"github.com/llm-proxy/llm-proxy/internal/api"
	"github.com/llm-proxy/llm-proxy/internal/config"
	"github.com/llm-proxy/llm-proxy/internal/logging"
	"github.com/llm-proxy/llm-proxy/internal/provider"
	"github.com/llm-proxy/llm-proxy/internal/provider/gemini"
	"github.com/llm-proxy/llm-proxy/internal/provider/groq"
	"github.com/llm-proxy/llm-proxy/internal/provider/openrouter"
	"github.com/llm-proxy/llm-proxy/internal/store"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		logging.Error("config_failed", "err", err.Error())
		os.Exit(1)
	}
	logging.Configure(cfg.LogLevel)
	logging.Info("startup", "log_level", cfg.LogLevel)

	st, err := store.Open(cfg.DatabaseURL)
	if err != nil {
		logging.Error("store_failed", "err", err.Error())
		os.Exit(1)
	}
	defer st.Close()
	logging.Info("database_ready", "driver", "postgres")

	var providers []provider.Provider
	if cfg.GeminiAPIKey != "" {
		providers = append(providers, gemini.New(cfg.GeminiAPIKey, cfg.GeminiBaseURL))
		logging.Info("provider_enabled", "provider", "gemini")
	}
	if cfg.GroqAPIKey != "" {
		providers = append(providers, groq.New(cfg.GroqAPIKey, cfg.GroqBaseURL))
		logging.Info("provider_enabled", "provider", "groq")
	}
	if cfg.OpenRouterAPIKey != "" {
		providers = append(providers, openrouter.New(
			cfg.OpenRouterAPIKey,
			cfg.OpenRouterBaseURL,
			cfg.OpenRouterSiteURL,
			cfg.OpenRouterTitle,
		))
		logging.Info("provider_enabled", "provider", "openrouter")
	}

	registry := provider.NewRegistry(st, providers...)
	srv := api.NewServer(registry, st, cfg.OpenAPIPath)
	logging.Info("listening", "addr", cfg.Addr, "docs", "/docs")
	if err := http.ListenAndServe(cfg.Addr, srv.Handler()); err != nil {
		logging.Error("server_failed", "err", err.Error())
		os.Exit(1)
	}
}
