package main

import (
	"log"
	"net/http"

	"github.com/llm-proxy/llm-proxy/internal/api"
	"github.com/llm-proxy/llm-proxy/internal/config"
	"github.com/llm-proxy/llm-proxy/internal/provider"
	"github.com/llm-proxy/llm-proxy/internal/provider/gemini"
	"github.com/llm-proxy/llm-proxy/internal/provider/groq"
	"github.com/llm-proxy/llm-proxy/internal/provider/openrouter"
	"github.com/llm-proxy/llm-proxy/internal/store"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	st, err := store.Open(cfg.DatabasePath)
	if err != nil {
		log.Fatalf("store: %v", err)
	}
	defer st.Close()
	log.Printf("database: %s", cfg.DatabasePath)

	var providers []provider.Provider
	if cfg.GeminiAPIKey != "" {
		providers = append(providers, gemini.New(cfg.GeminiAPIKey, cfg.GeminiBaseURL))
		log.Printf("provider enabled: gemini")
	}
	if cfg.GroqAPIKey != "" {
		providers = append(providers, groq.New(cfg.GroqAPIKey, cfg.GroqBaseURL))
		log.Printf("provider enabled: groq")
	}
	if cfg.OpenRouterAPIKey != "" {
		providers = append(providers, openrouter.New(
			cfg.OpenRouterAPIKey,
			cfg.OpenRouterBaseURL,
			cfg.OpenRouterSiteURL,
			cfg.OpenRouterTitle,
		))
		log.Printf("provider enabled: openrouter")
	}

	registry := provider.NewRegistry(st, providers...)
	srv := api.NewServer(registry, st, cfg.OpenAPIPath)
	log.Printf("llm-proxy listening on %s (docs: /docs)", cfg.Addr)
	if err := http.ListenAndServe(cfg.Addr, srv.Handler()); err != nil {
		log.Fatalf("server: %v", err)
	}
}
