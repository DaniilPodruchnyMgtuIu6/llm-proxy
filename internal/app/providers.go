package app

import (
	"github.com/llm-proxy/llm-proxy/internal/config"
	"github.com/llm-proxy/llm-proxy/internal/logging"
	"github.com/llm-proxy/llm-proxy/internal/provider"
	"github.com/llm-proxy/llm-proxy/internal/provider/gemini"
	"github.com/llm-proxy/llm-proxy/internal/provider/groq"
	"github.com/llm-proxy/llm-proxy/internal/provider/openrouter"
	"github.com/llm-proxy/llm-proxy/internal/runtime"
)

// BuildProviders constructs provider clients from effective keys + static base URLs.
func BuildProviders(cfg config.Config, keys runtime.Keys) []provider.Provider {
	var out []provider.Provider
	if keys.GeminiAPIKey != "" {
		out = append(out, gemini.New(keys.GeminiAPIKey, cfg.GeminiBaseURL))
		logging.Info("provider_enabled", "provider", "gemini")
	}
	if keys.GroqAPIKey != "" {
		out = append(out, groq.New(keys.GroqAPIKey, cfg.GroqBaseURL))
		logging.Info("provider_enabled", "provider", "groq")
	}
	if keys.OpenRouterAPIKey != "" {
		out = append(out, openrouter.New(
			keys.OpenRouterAPIKey,
			cfg.OpenRouterBaseURL,
			cfg.OpenRouterSiteURL,
			cfg.OpenRouterTitle,
		))
		logging.Info("provider_enabled", "provider", "openrouter")
	}
	return out
}

func EffectiveKeys(cfg config.Config, rt *runtime.Store) runtime.Keys {
	keys := runtime.Keys{
		GeminiAPIKey:     cfg.GeminiAPIKey,
		GroqAPIKey:       cfg.GroqAPIKey,
		OpenRouterAPIKey: cfg.OpenRouterAPIKey,
	}
	if rt == nil {
		return keys
	}
	st := rt.Get()
	// Runtime keys win when set (UI-managed).
	if st.Keys.GeminiAPIKey != "" {
		keys.GeminiAPIKey = st.Keys.GeminiAPIKey
	}
	if st.Keys.GroqAPIKey != "" {
		keys.GroqAPIKey = st.Keys.GroqAPIKey
	}
	if st.Keys.OpenRouterAPIKey != "" {
		keys.OpenRouterAPIKey = st.Keys.OpenRouterAPIKey
	}
	return keys
}
