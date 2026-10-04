package api

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/llm-proxy/llm-proxy/internal/provider"
	"github.com/llm-proxy/llm-proxy/internal/store"
)

type Server struct {
	registry    *provider.Registry
	store       *store.Store
	mux         *http.ServeMux
	openAPIPath string
}

func NewServer(registry *provider.Registry, st *store.Store, openAPIPath string) *Server {
	s := &Server{
		registry:    registry,
		store:       st,
		mux:         http.NewServeMux(),
		openAPIPath: openAPIPath,
	}
	s.routes()
	return s
}

func (s *Server) Handler() http.Handler {
	return logging(s.mux)
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /healthz", s.handleHealth)
	s.mux.HandleFunc("GET /docs", s.handleDocs)
	s.mux.HandleFunc("GET /docs/", s.handleDocs)
	s.mux.HandleFunc("GET /openapi.yaml", s.handleOpenAPI)
	s.mux.HandleFunc("GET /v1/providers", s.handleListProviders)
	s.mux.HandleFunc("GET /v1/models", s.handleListModels)
	s.mux.HandleFunc("GET /v1/stats/summary", s.handleStatsSummary)
	s.mux.HandleFunc("POST /v1/chat/completions", s.handleChatCompletions)
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleListProviders(w http.ResponseWriter, r *http.Request) {
	resp, err := s.registry.ListProviders(r.Context())
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
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
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
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
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

func (s *Server) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	var body json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	start := time.Now()
	respBody, status, providerName, modelID, err := s.registry.ChatCompletions(r.Context(), body)
	latency := time.Since(start).Milliseconds()

	ev := store.UsageEvent{
		TS:         time.Now().UTC(),
		Provider:   providerName,
		Model:      modelID,
		PoolID:     provider.ResolvePoolID(providerName, modelID),
		StatusCode: status,
		LatencyMS:  latency,
	}
	if status == 0 {
		status = http.StatusBadGateway
		ev.StatusCode = status
	}
	if err != nil {
		ev.ErrorType = classifyError(status, err)
	} else {
		fillUsageFromResponse(respBody, &ev)
	}
	if s.store != nil && providerName != "" {
		if recErr := s.store.RecordUsage(r.Context(), ev); recErr != nil {
			log.Printf("store record usage: %v", recErr)
		}
	}

	if err != nil {
		if len(respBody) > 0 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			_, _ = w.Write(respBody)
			return
		}
		writeError(w, status, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(respBody)
}

func fillUsageFromResponse(body json.RawMessage, ev *store.UsageEvent) {
	var parsed struct {
		ID    string `json:"id"`
		Usage *struct {
			PromptTokens     int64 `json:"prompt_tokens"`
			CompletionTokens int64 `json:"completion_tokens"`
			TotalTokens      int64 `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return
	}
	ev.RequestID = parsed.ID
	if parsed.Usage != nil {
		ev.PromptTokens = parsed.Usage.PromptTokens
		ev.CompletionTokens = parsed.Usage.CompletionTokens
		ev.TotalTokens = parsed.Usage.TotalTokens
	}
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

func logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &statusRecorder{ResponseWriter: w, status: 200}
		next.ServeHTTP(rw, r)
		log.Printf("%s %s %d %s", r.Method, r.URL.Path, rw.status, time.Since(start))
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
