package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/llm-proxy/llm-proxy/internal/ranking"
	"github.com/llm-proxy/llm-proxy/internal/store"
)

// Registry aggregates providers and routes chat requests by model id.
type Registry struct {
	mu        sync.RWMutex
	providers []Provider
	byModel   map[string]Provider
	store     *store.Store
}

// RouteOptions controls auto-routing / fallback candidate selection.
type RouteOptions struct {
	Provider        string
	RecommendedOnly bool
	ExcludeModels   []string
}

// ChatResult is the outcome of a (possibly multi-attempt) chat completion.
type ChatResult struct {
	Body     json.RawMessage
	Status   int
	Headers  http.Header
	Provider string
	Model    string
	Attempts int
	Tried    []string
}

func NewRegistry(st *store.Store, providers ...Provider) *Registry {
	return &Registry{
		providers: providers,
		byModel:   make(map[string]Provider),
		store:     st,
	}
}

func (r *Registry) ListModels(ctx context.Context, filter ListFilter) (ModelsResponse, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.byModel = make(map[string]Provider)
	out := ModelsResponse{Object: "list", Data: make([]Model, 0)}

	var firstErr error
	for _, p := range r.providers {
		models, err := p.ListModels(ctx)
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("%s: list models: %w", p.Name(), err)
			}
			continue
		}
		for _, m := range models {
			r.byModel[m.ID] = p
			m.QualityScore = ranking.Score(m.Provider, m.ID)
			r.applyLocalQuota(ctx, &m)
			r.applyRateLimitQuota(ctx, &m)
			if filter.Provider != "" && !strings.EqualFold(filter.Provider, p.Name()) {
				continue
			}
			if filter.FreeOnly && !m.Free {
				continue
			}
			if filter.Recommended && !m.Recommended {
				continue
			}
			out.Data = append(out.Data, m)
		}
	}
	if len(out.Data) == 0 && firstErr != nil {
		return ModelsResponse{}, firstErr
	}

	sort.SliceStable(out.Data, func(i, j int) bool {
		if out.Data[i].QualityScore != out.Data[j].QualityScore {
			return out.Data[i].QualityScore > out.Data[j].QualityScore
		}
		return out.Data[i].ID < out.Data[j].ID
	})
	for i := range out.Data {
		out.Data[i].Rank = i + 1
	}

	return out, nil
}

func (r *Registry) applyLocalQuota(ctx context.Context, m *Model) {
	if m.Quota == nil || r.store == nil {
		return
	}
	if m.Quota.Source == "live" && m.Quota.RemainingRPD != nil {
		return
	}
	if m.Quota.RPD == nil {
		return
	}

	day := store.DayBucket(m.Provider, time.Now())
	var used int64
	var err error
	if m.Quota.PoolID != "" {
		used, err = r.store.PoolSuccessCount(ctx, day, m.Quota.PoolID)
	} else {
		used, err = r.store.SuccessCount(ctx, day, m.Provider, m.ID, "")
	}
	if err != nil {
		return
	}

	rem := *m.Quota.RPD - used
	if rem < 0 {
		rem = 0
	}
	m.Quota.UsedRPD = &used
	m.Quota.RemainingRPD = &rem
	m.Quota.Source = "local_db"
	if m.Quota.Confidence == "" {
		m.Quota.Confidence = "estimate"
	}
}

func (r *Registry) applyRateLimitQuota(ctx context.Context, m *Model) {
	if r.store == nil || m == nil {
		return
	}
	rl, ok, err := r.store.GetRateLimit(ctx, m.Provider, m.ID)
	if err != nil || !ok {
		return
	}
	if m.Quota == nil {
		m.Quota = &Quota{Scope: "per_model", Confidence: "exact"}
	}
	if rl.LimitRequests != nil {
		m.Quota.RPM = rl.LimitRequests
	}
	if rl.RemainingRequests != nil {
		m.Quota.RemainingRPM = rl.RemainingRequests
	}
	if rl.LimitTokens != nil {
		m.Quota.TPM = rl.LimitTokens
	}
	if rl.RemainingTokens != nil {
		m.Quota.RemainingTPM = rl.RemainingTokens
	}
	if rl.RemainingRequests != nil || rl.RemainingTokens != nil {
		m.Quota.Source = "live_headers"
		m.Quota.Confidence = "exact"
		if rl.ResetRequests != "" {
			note := "rpm_reset=" + rl.ResetRequests
			if m.Quota.Notes == "" {
				m.Quota.Notes = note
			} else if !strings.Contains(m.Quota.Notes, "rpm_reset=") {
				m.Quota.Notes = m.Quota.Notes + "; " + note
			}
		}
	}
}

func (r *Registry) ListProviders(ctx context.Context) (ProvidersResponse, error) {
	out := ProvidersResponse{Object: "list", Data: make([]Status, 0, len(r.providers))}
	for _, p := range r.providers {
		st, err := p.Status(ctx)
		if err != nil {
			st = Status{
				ID:      p.Name(),
				Name:    p.DisplayName(),
				Enabled: true,
				Healthy: false,
				Error:   err.Error(),
				Docs:    p.DocsURL(),
			}
		}
		if st.Quota != nil && r.store != nil && !(st.Quota.Source == "live" && st.Quota.RemainingRPD != nil) {
			day := store.DayBucket(st.ID, time.Now())
			if st.Quota.PoolID != "" {
				if used, err := r.store.PoolSuccessCount(ctx, day, st.Quota.PoolID); err == nil && st.Quota.RPD != nil {
					rem := *st.Quota.RPD - used
					if rem < 0 {
						rem = 0
					}
					st.Quota.UsedRPD = &used
					st.Quota.RemainingRPD = &rem
					st.Quota.Source = "local_db"
				}
			}
		}
		out.Data = append(out.Data, st)
	}
	return out, nil
}

// ChatCompletions routes a chat request, with auto-select and fallback on 429/5xx.
func (r *Registry) ChatCompletions(ctx context.Context, body json.RawMessage, opts RouteOptions) (ChatResult, error) {
	var req struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return ChatResult{Status: 400}, fmt.Errorf("invalid request body: %w", err)
	}

	model := strings.TrimSpace(req.Model)
	auto := model == "" || strings.EqualFold(model, "auto")

	candidates, err := r.candidates(ctx, model, auto, opts)
	if err != nil {
		return ChatResult{Status: 502}, err
	}
	if len(candidates) == 0 {
		if auto {
			return ChatResult{Status: 503}, fmt.Errorf("no available models for routing")
		}
		return ChatResult{Status: 404}, fmt.Errorf("model not found: %s", model)
	}

	var last ChatResult
	var lastErr error
	for _, c := range candidates {
		attemptBody, err := setModel(body, c.ID)
		if err != nil {
			last = ChatResult{Status: 400, Provider: c.Provider, Model: c.ID}
			lastErr = err
			continue
		}

		r.mu.RLock()
		p, ok := r.byModel[c.ID]
		r.mu.RUnlock()
		if !ok {
			continue
		}

		resp, status, hdr, err := p.ChatCompletions(ctx, attemptBody)
		r.persistRateLimits(ctx, p.Name(), c.ID, hdr)

		tried := append(last.Tried, p.Name()+"/"+c.ID)
		result := ChatResult{
			Body:     resp,
			Status:   status,
			Headers:  hdr,
			Provider: p.Name(),
			Model:    c.ID,
			Attempts: len(tried),
			Tried:    tried,
		}
		last = result
		lastErr = err

		if err == nil && status >= 200 && status < 300 {
			return result, nil
		}
		if !isRetryableStatus(status) {
			return result, err
		}
		// retryable: continue to next candidate
	}

	if last.Status == 0 {
		last.Status = 503
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("all routing candidates failed")
	}
	return last, lastErr
}

func (r *Registry) candidates(ctx context.Context, model string, auto bool, opts RouteOptions) ([]Model, error) {
	filter := ListFilter{
		Provider:    opts.Provider,
		Recommended: opts.RecommendedOnly,
	}
	listed, err := r.ListModels(ctx, filter)
	if err != nil && len(listed.Data) == 0 {
		return nil, err
	}

	exclude := make(map[string]struct{}, len(opts.ExcludeModels))
	for _, id := range opts.ExcludeModels {
		exclude[strings.TrimSpace(id)] = struct{}{}
	}

	available := make([]Model, 0, len(listed.Data))
	for _, m := range listed.Data {
		if _, skip := exclude[m.ID]; skip {
			continue
		}
		if !isRoutable(m) {
			continue
		}
		available = append(available, m)
	}

	if auto {
		return available, nil
	}

	// Prefer requested model first, then remaining by rank.
	var primary *Model
	rest := make([]Model, 0, len(available))
	for i := range available {
		if available[i].ID == model {
			cp := available[i]
			primary = &cp
			continue
		}
		rest = append(rest, available[i])
	}
	if primary == nil {
		// Model may be filtered out as exhausted — still try it once if registered.
		r.mu.RLock()
		p, ok := r.byModel[model]
		r.mu.RUnlock()
		if !ok {
			return nil, nil
		}
		return []Model{{
			ID:       model,
			Provider: p.Name(),
			Object:   "model",
			OwnedBy:  p.Name(),
		}}, nil
	}
	out := make([]Model, 0, 1+len(rest))
	out = append(out, *primary)
	out = append(out, rest...)
	return out, nil
}

func isRoutable(m Model) bool {
	if m.Quota == nil {
		return true
	}
	if m.Quota.RemainingRPD != nil && *m.Quota.RemainingRPD <= 0 {
		return false
	}
	if m.Quota.RemainingRPM != nil && *m.Quota.RemainingRPM <= 0 {
		return false
	}
	return true
}

func isRetryableStatus(status int) bool {
	switch status {
	case 429, 502, 503, 504:
		return true
	default:
		return status >= 500
	}
}

func (r *Registry) persistRateLimits(ctx context.Context, providerName, modelID string, hdr http.Header) {
	if r.store == nil || hdr == nil {
		return
	}
	rl := store.ParseRateLimitHeaders(hdr)
	if rl.LimitRequests == nil && rl.RemainingRequests == nil &&
		rl.LimitTokens == nil && rl.RemainingTokens == nil {
		return
	}
	rl.Provider = providerName
	rl.Model = modelID
	_ = r.store.UpsertRateLimit(ctx, rl)
}

func setModel(body json.RawMessage, model string) (json.RawMessage, error) {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(body, &obj); err != nil {
		return nil, err
	}
	raw, err := json.Marshal(model)
	if err != nil {
		return nil, err
	}
	obj["model"] = raw
	return json.Marshal(obj)
}

// ResolvePoolID best-effort: openrouter free pool, else empty.
func ResolvePoolID(providerName, modelID string) string {
	if providerName == "openrouter" {
		return "openrouter:free"
	}
	return ""
}
