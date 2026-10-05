package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/llm-proxy/llm-proxy/internal/logging"
	"github.com/llm-proxy/llm-proxy/internal/provider"
	"github.com/llm-proxy/llm-proxy/internal/runtime"
	"github.com/llm-proxy/llm-proxy/internal/store"
)

func (s *Server) handleAdminListPresets(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		writeError(w, http.StatusServiceUnavailable, "store not configured")
		return
	}
	list, err := s.store.ListPresets(r.Context())
	if err != nil {
		logging.Errorf(r.Context(), "presets_list_failed", "err", err.Error())
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	logging.Debugf(r.Context(), "presets_list_ok", "count", len(list))
	writeJSON(w, http.StatusOK, map[string]any{"object": "list", "data": list})
}

func (s *Server) handleAdminGetPreset(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	p, err := s.store.GetPreset(r.Context(), slug)
	if err != nil {
		writePresetErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s.presetResponse(r, p))
}

func (s *Server) handleAdminCreatePreset(w http.ResponseWriter, r *http.Request) {
	var body store.Preset
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	body.IsSystem = false
	if err := validatePresetFields(body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	p, err := s.store.CreatePreset(r.Context(), body)
	if err != nil {
		writePresetErr(w, err)
		return
	}
	logging.Infof(r.Context(), "preset_created", "slug", p.Slug, "model", p.Model)
	writeJSON(w, http.StatusCreated, s.presetResponse(r, p))
}

func (s *Server) handleAdminUpdatePreset(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	var body store.Preset
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if err := validatePresetFields(body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	p, err := s.store.UpdatePreset(r.Context(), slug, body)
	if err != nil {
		writePresetErr(w, err)
		return
	}
	if p.Slug == "default" && s.runtime != nil {
		st := s.runtime.Get()
		st.Defaults = runtime.Defaults{
			Model:            p.Model,
			Temperature:      p.Temperature,
			TopP:             p.TopP,
			TopK:             p.TopK,
			MaxTokens:        p.MaxTokens,
			PresencePenalty:  p.PresencePenalty,
			FrequencyPenalty: p.FrequencyPenalty,
		}
		if err := s.runtime.Save(st); err != nil {
			logging.Errorf(r.Context(), "preset_default_runtime_sync_failed", "err", err.Error())
		}
	}
	logging.Infof(r.Context(), "preset_updated", "slug", p.Slug, "model", p.Model)
	writeJSON(w, http.StatusOK, s.presetResponse(r, p))
}

func (s *Server) handleAdminDeletePreset(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	if err := s.store.DeletePreset(r.Context(), slug); err != nil {
		writePresetErr(w, err)
		return
	}
	logging.Infof(r.Context(), "preset_deleted", "slug", slug)
	writeJSON(w, http.StatusOK, map[string]any{"deleted": true, "slug": slug})
}

func (s *Server) handlePresetChatCompletions(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	defer r.Body.Close()

	var body json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		logging.Errorf(r.Context(), "preset_chat_bad_json", "slug", slug, "err", err.Error())
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	merged, err := s.mergePresetBody(r, slug, body)
	if err != nil {
		writePresetErr(w, err)
		return
	}
	logging.Infof(r.Context(), "preset_chat_start", "slug", slug)
	s.serveChat(w, r, merged, provider.RouteOptions{}, slug)
}

func (s *Server) mergePresetBody(r *http.Request, slug string, body json.RawMessage) (json.RawMessage, error) {
	if s.store == nil {
		return nil, errors.New("store not configured")
	}
	p, err := s.store.GetPreset(r.Context(), slug)
	if err != nil {
		return nil, err
	}
	merged, err := provider.MergePresetIntoBody(body, provider.PresetFields{
		Model:            p.Model,
		Temperature:      p.Temperature,
		TopP:             p.TopP,
		TopK:             p.TopK,
		MaxTokens:        p.MaxTokens,
		PresencePenalty:  p.PresencePenalty,
		FrequencyPenalty: p.FrequencyPenalty,
	})
	if err != nil {
		return nil, err
	}
	if err := provider.ValidateChatBody(merged); err != nil {
		logging.Infof(r.Context(), "preset_chat_validation_failed", "slug", slug, "err", err.Error())
		return nil, err
	}
	logging.Debugf(r.Context(), "preset_merged", "slug", slug, "model", p.Model)
	return merged, nil
}

func validatePresetFields(p store.Preset) error {
	model := strings.TrimSpace(p.Model)
	if model == "" {
		model = "auto"
	}
	return provider.ValidateDefaults(
		model,
		p.Temperature, p.TopP, p.PresencePenalty, p.FrequencyPenalty,
		p.TopK, p.MaxTokens,
	)
}

func (s *Server) presetResponse(r *http.Request, p store.Preset) map[string]any {
	base := publicBaseURL(r)
	path := "/v1/p/" + p.Slug + "/chat/completions"
	url := strings.TrimRight(base, "/") + path
	example := "curl -s " + url + " \\\n  -H \"Content-Type: application/json\" \\\n  -d '{\"messages\":[{\"role\":\"user\",\"content\":\"ping\"}]}'"
	return map[string]any{
		"preset":       p,
		"endpoint":     path,
		"url":          url,
		"curl_example": example,
	}
}

func publicBaseURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		scheme = "https"
	}
	host := r.Host
	if host == "" {
		host = "localhost:8080"
	}
	return scheme + "://" + host
}

func writePresetErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrPresetNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, store.ErrPresetExists):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, store.ErrPresetSystemProtect):
		writeError(w, http.StatusForbidden, err.Error())
	case errors.Is(err, store.ErrPresetInvalidSlug):
		writeError(w, http.StatusBadRequest, err.Error())
	default:
		// validation errors from ValidateDefaults / ValidateChatBody
		msg := err.Error()
		if strings.Contains(msg, "must be") || strings.Contains(msg, "too small") || strings.Contains(msg, "invalid") {
			writeError(w, http.StatusBadRequest, msg)
			return
		}
		writeError(w, http.StatusInternalServerError, msg)
	}
}
