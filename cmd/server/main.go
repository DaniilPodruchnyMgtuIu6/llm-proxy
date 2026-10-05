package main

import (
	"context"
	"net/http"
	"os"

	"github.com/llm-proxy/llm-proxy/internal/api"
	"github.com/llm-proxy/llm-proxy/internal/app"
	"github.com/llm-proxy/llm-proxy/internal/config"
	"github.com/llm-proxy/llm-proxy/internal/logging"
	"github.com/llm-proxy/llm-proxy/internal/provider"
	"github.com/llm-proxy/llm-proxy/internal/runtime"
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
		"runtime_dir", cfg.RuntimeDataDir,
	)

	st, err := store.Open(cfg.DatabaseURL)
	if err != nil {
		logging.Error("store_failed", "err", err.Error())
		os.Exit(1)
	}
	defer st.Close()
	logging.Info("database_ready", "driver", "postgres")

	rt, err := runtime.Open(cfg.RuntimeDataDir)
	if err != nil {
		logging.Error("runtime_failed", "err", err.Error())
		os.Exit(1)
	}
	if merged, err := rt.MergeEnvKeys(cfg.GeminiAPIKey, cfg.GroqAPIKey, cfg.OpenRouterAPIKey); err != nil {
		logging.Error("runtime_merge_failed", "err", err.Error())
		os.Exit(1)
	} else if merged {
		logging.Info("runtime_merged_env_keys")
	}

	seed := store.Preset{Model: "auto"}
	if d := rt.Get().Defaults; d.Model != "" || d.Temperature != nil || d.TopP != nil || d.MaxTokens != nil {
		seed.Model = d.Model
		if seed.Model == "" {
			seed.Model = "auto"
		}
		seed.Temperature = d.Temperature
		seed.TopP = d.TopP
		seed.TopK = d.TopK
		seed.MaxTokens = d.MaxTokens
		seed.PresencePenalty = d.PresencePenalty
		seed.FrequencyPenalty = d.FrequencyPenalty
	}
	if err := st.EnsureDefaultPreset(context.Background(), seed); err != nil {
		logging.Error("preset_default_seed_failed", "err", err.Error())
		os.Exit(1)
	}
	logging.Info("preset_default_ready", "model", seed.Model)

	keys := app.EffectiveKeys(cfg, rt)
	providers := app.BuildProviders(cfg, keys)
	if len(providers) == 0 {
		logging.Info("waiting_for_keys", "hint", "open UI setup wizard or set keys via PUT /admin/keys")
	}

	regOpts := provider.Options{
		UpstreamTimeout: cfg.UpstreamTimeout,
		MaxAttempts:     cfg.MaxFallbackAttempts,
		ModelsCacheTTL:  cfg.ModelsCacheTTL,
		CircuitErrors:   cfg.CircuitErrors,
		CircuitCooldown: cfg.CircuitCooldown,
	}
	registry := provider.NewRegistryWithOptions(st, regOpts, providers...)
	srv := api.NewServerWithOptions(api.ServerOptions{
		Registry:    registry,
		Store:       st,
		Runtime:     rt,
		Config:      cfg,
		OpenAPIPath: cfg.OpenAPIPath,
		APIKey:      cfg.ProxyAPIKey,
	})
	logging.Info("listening", "addr", cfg.Addr, "docs", "/docs", "admin", "/admin/status")
	if err := http.ListenAndServe(cfg.Addr, srv.Handler()); err != nil {
		logging.Error("server_failed", "err", err.Error())
		os.Exit(1)
	}
}
