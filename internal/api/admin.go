package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/llm-proxy/llm-proxy/internal/app"
	"github.com/llm-proxy/llm-proxy/internal/logging"
	"github.com/llm-proxy/llm-proxy/internal/provider"
	"github.com/llm-proxy/llm-proxy/internal/runtime"
)

func (s *Server) handleAdminStatus(w http.ResponseWriter, r *http.Request) {
	st := runtime.State{Defaults: runtime.Defaults{Model: "auto"}}
	if s.runtime != nil {
		st = s.runtime.Get()
	}
	keys := app.EffectiveKeys(s.cfg, s.runtime)
	writeJSON(w, http.StatusOK, map[string]any{
		"setup_complete": keys.GeminiAPIKey != "" || keys.GroqAPIKey != "" || keys.OpenRouterAPIKey != "",
		"providers": map[string]any{
			"gemini": map[string]any{
				"configured": keys.GeminiAPIKey != "",
				"masked_key": runtime.MaskKey(keys.GeminiAPIKey),
			},
			"groq": map[string]any{
				"configured": keys.GroqAPIKey != "",
				"masked_key": runtime.MaskKey(keys.GroqAPIKey),
			},
			"openrouter": map[string]any{
				"configured": keys.OpenRouterAPIKey != "",
				"masked_key": runtime.MaskKey(keys.OpenRouterAPIKey),
			},
		},
		"defaults":      st.Defaults,
		"provider_count": s.registry.ProviderCount(),
		"runtime_path":  s.runtimePath(),
	})
}

func (s *Server) handleAdminKeys(w http.ResponseWriter, r *http.Request) {
	if s.runtime == nil {
		writeError(w, http.StatusServiceUnavailable, "runtime store not configured")
		return
	}
	var body struct {
		GeminiAPIKey     *string `json:"gemini_api_key"`
		GroqAPIKey       *string `json:"groq_api_key"`
		OpenRouterAPIKey *string `json:"openrouter_api_key"`
		Apply            *bool   `json:"apply"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	st := s.runtime.Get()
	if body.GeminiAPIKey != nil {
		st.Keys.GeminiAPIKey = strings.TrimSpace(*body.GeminiAPIKey)
	}
	if body.GroqAPIKey != nil {
		st.Keys.GroqAPIKey = strings.TrimSpace(*body.GroqAPIKey)
	}
	if body.OpenRouterAPIKey != nil {
		st.Keys.OpenRouterAPIKey = strings.TrimSpace(*body.OpenRouterAPIKey)
	}
	if st.Keys.GeminiAPIKey == "" && st.Keys.GroqAPIKey == "" && st.Keys.OpenRouterAPIKey == "" {
		// keep env-only setup valid: allow clearing runtime keys
	}
	if err := s.runtime.Save(st); err != nil {
		logging.Errorf(r.Context(), "admin_keys_save_failed", "err", err.Error())
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	apply := true
	if body.Apply != nil {
		apply = *body.Apply
	}
	if apply {
		if err := s.reloadProviders(r); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	logging.Infof(r.Context(), "admin_keys_updated", "apply", apply)
	s.handleAdminStatus(w, r)
}

func (s *Server) handleAdminDefaults(w http.ResponseWriter, r *http.Request) {
	if s.runtime == nil {
		writeError(w, http.StatusServiceUnavailable, "runtime store not configured")
		return
	}
	var body runtime.Defaults
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if strings.TrimSpace(body.Model) == "" {
		body.Model = "auto"
	}
	if err := provider.ValidateDefaults(
		body.Model,
		body.Temperature, body.TopP, body.PresencePenalty, body.FrequencyPenalty,
		body.TopK, body.MaxTokens,
	); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	st := s.runtime.Get()
	st.Defaults = body
	if err := s.runtime.Save(st); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	logging.Infof(r.Context(), "admin_defaults_updated", "model", st.Defaults.Model)
	writeJSON(w, http.StatusOK, map[string]any{"defaults": st.Defaults})
}

func (s *Server) handleAdminApply(w http.ResponseWriter, r *http.Request) {
	if err := s.reloadProviders(r); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	logging.Infof(r.Context(), "admin_apply_ok", "providers", s.registry.ProviderCount())
	s.handleAdminStatus(w, r)
}

func (s *Server) reloadProviders(r *http.Request) error {
	keys := app.EffectiveKeys(s.cfg, s.runtime)
	providers := app.BuildProviders(s.cfg, keys)
	s.registry.ReplaceProviders(providers...)
	logging.Infof(r.Context(), "providers_reloaded", "count", len(providers))
	return nil
}

func (s *Server) runtimePath() string {
	if s.runtime == nil {
		return ""
	}
	return s.runtime.Path()
}
