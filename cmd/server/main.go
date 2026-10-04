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
	logging.Info("startup",
		"log_level", cfg.LogLevel,
		"upstream_timeout", cfg.UpstreamTimeout.String(),
		"max_fallback_attempts", cfg.MaxFallbackAttempts,
		"models_cache_ttl", cfg.ModelsCacheTTL.String(),
		"proxy_auth", cfg.ProxyAPIKey != "",
	)

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

	regOpts := provider.Options{
		UpstreamTimeout: cfg.UpstreamTimeout,
		MaxAttempts:     cfg.MaxFallbackAttempts,
		ModelsCacheTTL:  cfg.ModelsCacheTTL,
		CircuitErrors:   cfg.CircuitErrors,
		CircuitCooldown: cfg.CircuitCooldown,
	}
	registry := provider.NewRegistryWithOptions(st, regOpts, providers...)
	srv := api.NewServerWithAuth(registry, st, cfg.OpenAPIPath, cfg.ProxyAPIKey)
	logging.Info("listening", "addr", cfg.Addr, "docs", "/docs")
	if err := http.ListenAndServe(cfg.Addr, srv.Handler()); err != nil {
		logging.Error("server_failed", "err", err.Error())
		os.Exit(1)
	}
}
