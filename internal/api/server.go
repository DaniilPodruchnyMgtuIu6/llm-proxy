package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/llm-proxy/llm-proxy/internal/logging"
	"github.com/llm-proxy/llm-proxy/internal/provider"
	"github.com/llm-proxy/llm-proxy/internal/store"
)

type Server struct {
	registry    *provider.Registry
	store       *store.Store
	mux         *http.ServeMux
	openAPIPath string
	apiKey      string
}

func NewServer(registry *provider.Registry, st *store.Store, openAPIPath string) *Server {
	return NewServerWithAuth(registry, st, openAPIPath, "")
}

func NewServerWithAuth(registry *provider.Registry, st *store.Store, openAPIPath, apiKey string) *Server {
	s := &Server{
		registry:    registry,
		store:       st,
		mux:         http.NewServeMux(),
		openAPIPath: openAPIPath,
		apiKey:      strings.TrimSpace(apiKey),
	}
	s.routes()
	return s
}

func (s *Server) Handler() http.Handler {
	return withRequestContext(withAPIKey(s.apiKey, s.mux))
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /healthz", s.handleHealth)
	s.mux.HandleFunc("GET /docs", s.handleDocs)
	s.mux.HandleFunc("GET /docs/", s.handleDocs)
	s.mux.HandleFunc("GET /architecture", s.handleArchitecture)
	s.mux.HandleFunc("GET /architecture/", s.handleArchitecture)
	s.mux.HandleFunc("GET /architecture.md", s.handleArchitectureMarkdown)
	s.mux.HandleFunc("GET /openapi.yaml", s.handleOpenAPI)
	s.mux.HandleFunc("GET /v1/providers", s.handleListProviders)
	s.mux.HandleFunc("GET /v1/models", s.handleListModels)
	s.mux.HandleFunc("GET /v1/stats/summary", s.handleStatsSummary)
	s.mux.HandleFunc("POST /v1/chat/completions", s.handleChatCompletions)
	s.mux.HandleFunc("POST /v1/completions", s.handleChatCompletions)
	s.mux.HandleFunc("POST /v1/completion", s.handleChatCompletions)
	s.mux.HandleFunc("POST /v1/route", s.handleRoute)
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleListProviders(w http.ResponseWriter, r *http.Request) {
	resp, err := s.registry.ListProviders(r.Context())
	if err != nil {
		logging.Errorf(r.Context(), "list_providers_failed", "err", err.Error())
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	logging.Debugf(r.Context(), "list_providers_ok", "count", len(resp.Data))
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleListModels(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	filter := provider.ListFilter{
		Provider:    strings.TrimSpace(q.Get("provider")),
		FreeOnly:    parseBool(q.Get("free")),
		Recommended: parseBool(q.Get("recommended")),
	}

	resp, err := s.registry.ListModels(r.Context(), filter)
	if err != nil {
		logging.Errorf(r.Context(), "list_models_failed", "err", err.Error())
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	logging.Debugf(r.Context(), "list_models_ok", "count", len(resp.Data), "provider", filter.Provider)
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleStatsSummary(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	now := time.Now().UTC()
	to := strings.TrimSpace(q.Get("to"))
	from := strings.TrimSpace(q.Get("from"))
	if to == "" {
		to = now.Format("2006-01-02")
	}
	if from == "" {
		from = now.AddDate(0, 0, -7).Format("2006-01-02")
	}
	providerName := strings.TrimSpace(q.Get("provider"))

	summary, err := s.store.Summary(r.Context(), from, to, providerName)
	if err != nil {
		logging.Errorf(r.Context(), "stats_summary_failed", "err", err.Error())
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	logging.Debugf(r.Context(), "stats_summary_ok", "from", from, "to", to)
	writeJSON(w, http.StatusOK, summary)
}

func (s *Server) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	var body json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		logging.Errorf(r.Context(), "chat_bad_json", "err", err.Error())
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	s.serveChat(w, r, body, provider.RouteOptions{})
}

func (s *Server) handleRoute(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	var raw map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		logging.Errorf(r.Context(), "route_bad_json", "err", err.Error())
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	opts := provider.RouteOptions{}
	if v, ok := raw["provider"]; ok {
		_ = json.Unmarshal(v, &opts.Provider)
		opts.Provider = strings.TrimSpace(opts.Provider)
		delete(raw, "provider")
	}
	if v, ok := raw["recommended_only"]; ok {
		_ = json.Unmarshal(v, &opts.RecommendedOnly)
		delete(raw, "recommended_only")
	}
	if v, ok := raw["exclude_models"]; ok {
		_ = json.Unmarshal(v, &opts.ExcludeModels)
		delete(raw, "exclude_models")
	}
	modelJSON, _ := json.Marshal("auto")
	raw["model"] = modelJSON

	body, err := json.Marshal(raw)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid route body")
		return
	}
	logging.Debugf(r.Context(), "route_options", "provider", opts.Provider, "recommended_only", opts.RecommendedOnly)
	s.serveChat(w, r, body, opts)
}

func (s *Server) serveChat(w http.ResponseWriter, r *http.Request, body json.RawMessage, opts provider.RouteOptions) {
	ctx := r.Context()
	start := time.Now()
	result, err := s.registry.ChatCompletions(ctx, body, opts)
	latency := time.Since(start).Milliseconds()

	status := result.Status
	if status == 0 {
		status = http.StatusBadGateway
	}

	reqID := logging.RequestID(ctx)
	if len(result.Attempted) > 0 {
		for i, a := range result.Attempted {
			ev := store.UsageEvent{
				TS:         time.Now().UTC(),
				Provider:   a.Provider,
				Model:      a.Model,
				PoolID:     provider.ResolvePoolID(a.Provider, a.Model),
				StatusCode: a.Status,
				LatencyMS:  a.LatencyMS,
				RequestID:  reqID,
				ErrorType:  a.ErrorType,
			}
			if a.Status >= 200 && a.Status < 300 {
				upstreamID := fillUsageFromResponse(a.Body, &ev)
				if upstreamID != "" && i == len(result.Attempted)-1 {
					logging.Debugf(ctx, "upstream_response", "upstream_id", upstreamID, "provider", a.Provider, "model", a.Model)
				}
			}
			if s.store != nil && a.Provider != "" {
				if recErr := s.store.RecordUsage(ctx, ev); recErr != nil {
					logging.Errorf(ctx, "store_record_usage_failed", "err", recErr.Error(), "attempt", i+1)
				}
			}
		}
	} else if result.Provider != "" {
		// Rare path: failed before any upstream attempt.
		ev := store.UsageEvent{
			TS: time.Now().UTC(), Provider: result.Provider, Model: result.Model,
			PoolID: provider.ResolvePoolID(result.Provider, result.Model),
			StatusCode: status, LatencyMS: latency, RequestID: reqID,
		}
		if err != nil {
			ev.ErrorType = classifyError(status, err)
		}
		if s.store != nil {
			_ = s.store.RecordUsage(ctx, ev)
		}
	}

	if result.Model != "" {
		w.Header().Set("X-LLM-Proxy-Model", result.Model)
	}
	if result.Provider != "" {
		w.Header().Set("X-LLM-Proxy-Provider", result.Provider)
	}
	if result.Attempts > 0 {
		w.Header().Set("X-LLM-Proxy-Attempts", strconv.Itoa(result.Attempts))
	}
	if len(result.Tried) > 0 {
		w.Header().Set("X-LLM-Proxy-Tried", strings.Join(result.Tried, ","))
	}

	if err != nil {
		logging.Infof(ctx, "chat_done",
			"status", status,
			"provider", result.Provider,
			"model", result.Model,
			"attempts", result.Attempts,
			"latency_ms", latency,
			"error", err.Error(),
		)
		if len(result.Body) > 0 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			_, _ = w.Write(result.Body)
			return
		}
		writeError(w, status, err.Error())
		return
	}

	var tokens int64
	{
		var tmp store.UsageEvent
		_ = fillUsageFromResponse(result.Body, &tmp)
		tokens = tmp.TotalTokens
	}
	logging.Infof(ctx, "chat_done",
		"status", status,
		"provider", result.Provider,
		"model", result.Model,
		"attempts", result.Attempts,
		"latency_ms", latency,
		"tokens", tokens,
	)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(result.Body)
}

func fillUsageFromResponse(body json.RawMessage, ev *store.UsageEvent) string {
	var parsed struct {
		ID    string `json:"id"`
		Usage *struct {
			PromptTokens     int64 `json:"prompt_tokens"`
			CompletionTokens int64 `json:"completion_tokens"`
			TotalTokens      int64 `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return ""
	}
	if parsed.Usage != nil {
		ev.PromptTokens = parsed.Usage.PromptTokens
		ev.CompletionTokens = parsed.Usage.CompletionTokens
		ev.TotalTokens = parsed.Usage.TotalTokens
	}
	return parsed.ID
}

func classifyError(status int, err error) string {
	if status == 429 {
		return "rate_limit"
	}
	if status == 402 {
		return "credits"
	}
	if status == 404 {
		return "not_found"
	}
	if err != nil {
		return "upstream"
	}
	return ""
}

func parseBool(v string) bool {
	if v == "" {
		return false
	}
	b, err := strconv.ParseBool(v)
	return err == nil && b
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]any{
			"message": message,
			"type":    "proxy_error",
		},
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}
