package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/llm-proxy/llm-proxy/internal/logging"
	"github.com/llm-proxy/llm-proxy/internal/ranking"
	"github.com/llm-proxy/llm-proxy/internal/store"
)

// Options tunes routing reliability behaviour.
type Options struct {
	UpstreamTimeout time.Duration
	MaxAttempts     int
	ModelsCacheTTL  time.Duration
	CircuitErrors   int64
	CircuitCooldown time.Duration
}

func DefaultOptions() Options {
	return Options{
		UpstreamTimeout: 60 * time.Second,
		MaxAttempts:     3,
		ModelsCacheTTL:  45 * time.Second,
		CircuitErrors:   5,
		CircuitCooldown: 5 * time.Minute,
	}
}

// Registry aggregates providers and routes chat requests by model id.
type Registry struct {
	mu        sync.RWMutex
	providers []Provider
	byModel   map[string]Provider
	store     *store.Store
	opts      Options

	cacheMu    sync.Mutex
	cacheAt    time.Time
	cacheAll   []Model
	cacheIndex map[string]Provider

	denyMu    sync.Mutex
	denyUntil map[string]time.Time // model id temporarily unfit for auto-route
}

// RouteOptions controls auto-routing / fallback candidate selection.
type RouteOptions struct {
	Provider        string
	RecommendedOnly bool
	ExcludeModels   []string
}

// AttemptRecord is one upstream try (for usage/stats).
type AttemptRecord struct {
	Provider   string
	Model      string
	Status     int
	LatencyMS  int64
	ErrorType  string
	Body       json.RawMessage
}

// ChatResult is the outcome of a (possibly multi-attempt) chat completion.
type ChatResult struct {
	Body      json.RawMessage
	Status    int
	Headers   http.Header
	Provider  string
	Model     string
	Attempts  int
	Tried     []string
	Attempted []AttemptRecord
}

func NewRegistry(st *store.Store, providers ...Provider) *Registry {
	return NewRegistryWithOptions(st, DefaultOptions(), providers...)
}

func NewRegistryWithOptions(st *store.Store, opts Options, providers ...Provider) *Registry {
	if opts.MaxAttempts < 1 {
		opts.MaxAttempts = 1
	}
	if opts.UpstreamTimeout < time.Second {
		opts.UpstreamTimeout = time.Second
	}
	return &Registry{
		providers:  providers,
		byModel:    make(map[string]Provider),
		store:      st,
		opts:       opts,
		cacheIndex: make(map[string]Provider),
		denyUntil:  make(map[string]time.Time),
	}
}

// ReplaceProviders hot-swaps upstream clients and drops the models cache.
func (r *Registry) ReplaceProviders(providers ...Provider) {
	r.cacheMu.Lock()
	r.cacheAll = nil
	r.cacheIndex = make(map[string]Provider)
	r.cacheAt = time.Time{}
	r.cacheMu.Unlock()

	r.mu.Lock()
	r.providers = providers
	r.byModel = make(map[string]Provider)
	r.mu.Unlock()
}

func (r *Registry) ProviderCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.providers)
}

func (r *Registry) ListModels(ctx context.Context, filter ListFilter) (ModelsResponse, error) {
	all, err := r.cachedModels(ctx)
	if err != nil && len(all) == 0 {
		return ModelsResponse{}, err
	}

	out := ModelsResponse{Object: "list", Data: make([]Model, 0, len(all))}
	for _, m := range all {
		if filter.Provider != "" && !strings.EqualFold(filter.Provider, m.Provider) {
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
	for i := range out.Data {
		out.Data[i].Rank = i + 1
	}
	return out, nil
}

func (r *Registry) cachedModels(ctx context.Context) ([]Model, error) {
	r.cacheMu.Lock()
	defer r.cacheMu.Unlock()

	if r.opts.ModelsCacheTTL > 0 && time.Since(r.cacheAt) < r.opts.ModelsCacheTTL && len(r.cacheAll) > 0 {
		logging.Debugf(ctx, "models_cache_hit", "age_ms", time.Since(r.cacheAt).Milliseconds(), "count", len(r.cacheAll))
		r.mu.Lock()
		r.byModel = r.cacheIndex
		r.mu.Unlock()
		return r.cacheAll, nil
	}

	all, index, err := r.fetchAllModels(ctx)
	if err != nil && len(all) == 0 {
		return nil, err
	}
	r.cacheAll = all
	r.cacheIndex = index
	r.cacheAt = time.Now()
	r.mu.Lock()
	r.byModel = index
	r.mu.Unlock()
	logging.Debugf(ctx, "models_cache_refresh", "count", len(all), "ttl", r.opts.ModelsCacheTTL.String())
	return all, err
}

func (r *Registry) fetchAllModels(ctx context.Context) ([]Model, map[string]Provider, error) {
	index := make(map[string]Provider)
	out := make([]Model, 0)
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
			if !IsChatCapable(p.Name(), m.ID) {
				continue
			}
			index[m.ID] = p
			m.QualityScore = ranking.Score(m.Provider, m.ID)
			r.applyLocalQuota(ctx, &m)
			r.applyRateLimitQuota(ctx, &m)
			out = append(out, m)
		}
	}
	if len(out) == 0 && firstErr != nil {
		return nil, index, firstErr
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].QualityScore != out[j].QualityScore {
			return out[i].QualityScore > out[j].QualityScore
		}
		return out[i].ID < out[j].ID
	})
	return out, index, nil
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
		if r.providerOpenCircuit(ctx, p.Name()) {
			st.Healthy = false
			if st.Error == "" {
				st.Error = "circuit_open"
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
		r.enrichProviderQuotaFromModels(ctx, &st)
		out.Data = append(out.Data, st)
	}
	return out, nil
}

// enrichProviderQuotaFromModels fills remaining_rpd for per-model providers
// (Gemini/Groq) by summing chat-model leftovers — OpenRouter already has a live pool.
func (r *Registry) enrichProviderQuotaFromModels(ctx context.Context, st *Status) {
	if st == nil {
		return
	}
	if st.Quota != nil && st.Quota.RemainingRPD != nil && st.Quota.Scope == "shared_pool" {
		return
	}
	if st.Quota != nil && st.Quota.Source == "live" && st.Quota.RemainingRPD != nil {
		return
	}
	listed, err := r.ListModels(ctx, ListFilter{Provider: st.ID})
	if err != nil && len(listed.Data) == 0 {
		return
	}
	var sumRem, sumUsed, sumCap int64
	n := 0
	for _, m := range listed.Data {
		if m.Quota == nil || m.Quota.RemainingRPD == nil {
			continue
		}
		sumRem += *m.Quota.RemainingRPD
		if m.Quota.UsedRPD != nil {
			sumUsed += *m.Quota.UsedRPD
		}
		if m.Quota.RPD != nil {
			sumCap += *m.Quota.RPD
		}
		n++
	}
	if n == 0 {
		return
	}
	if st.Quota == nil {
		st.Quota = &Quota{Tier: "free", Confidence: "estimate"}
	}
	st.Quota.RemainingRPD = &sumRem
	st.Quota.UsedRPD = &sumUsed
	if sumCap > 0 {
		st.Quota.RPD = &sumCap
	}
	st.Quota.Scope = "per_model_aggregate"
	st.Quota.Source = "local_aggregate"
	st.Quota.Notes = fmt.Sprintf("Сумма remaining RPD по %d chat-моделям (у каждой свой лимит).", n)
}

// ChatCompletions routes a chat request, with auto-select and fallback on 429/5xx.
func (r *Registry) ChatCompletions(ctx context.Context, body json.RawMessage, opts RouteOptions) (ChatResult, error) {
	var req struct {
		Model    string          `json:"model"`
		Messages json.RawMessage `json:"messages"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return ChatResult{Status: 400}, fmt.Errorf("invalid request body: %w", err)
	}
	if len(req.Messages) == 0 || string(req.Messages) == "null" {
		return ChatResult{Status: 400}, fmt.Errorf("messages is required")
	}

	model := strings.TrimSpace(req.Model)
	auto := model == "" || strings.EqualFold(model, "auto")
	requested := model
	if auto {
		requested = "auto"
	}

	candidates, why, err := r.candidates(ctx, model, auto, opts)
	if err != nil {
		logging.Errorf(ctx, "route_candidates_failed", "requested", requested, "err", err.Error())
		return ChatResult{Status: 502}, err
	}
	if len(candidates) == 0 {
		logging.Infof(ctx, "route_no_candidates", "requested", requested, "reason", why)
		if auto {
			return ChatResult{Status: 503}, fmt.Errorf("no available models for routing")
		}
		switch why {
		case "circuit_open":
			return ChatResult{Status: 503}, fmt.Errorf("provider temporarily unavailable for model %s (circuit open after repeated errors; retry shortly)", model)
		case "quota_exhausted":
			return ChatResult{Status: 429}, fmt.Errorf("model temporarily unavailable (quota exhausted): %s", model)
		default:
			return ChatResult{Status: 404}, fmt.Errorf("model not found: %s", model)
		}
	}

	logging.Debugf(ctx, "route_start",
		"requested", requested,
		"candidates", len(candidates),
		"max_attempts", r.opts.MaxAttempts,
		"timeout", r.opts.UpstreamTimeout.String(),
	)

	var last ChatResult
	var lastErr error
	failedProviders := map[string]struct{}{}
	emptyRetried := map[string]struct{}{}
	attemptsStarted := 0
	for i, c := range candidates {
		if attemptsStarted >= r.opts.MaxAttempts {
			logging.Debugf(ctx, "route_max_attempts_reached", "max", r.opts.MaxAttempts)
			break
		}
		r.mu.RLock()
		p, ok := r.byModel[c.ID]
		r.mu.RUnlock()
		if !ok {
			continue
		}
		if _, skip := failedProviders[p.Name()]; skip {
			logging.Debugf(ctx, "route_skip_provider_failed", "provider", p.Name(), "model", c.ID)
			continue
		}
		// Explicit primary model may bypass circuit once; auto-route still respects it.
		forcedPrimary := !auto && i == 0 && c.ID == model
		if !forcedPrimary && r.providerOpenCircuit(ctx, p.Name()) {
			logging.Infof(ctx, "route_skip_circuit", "provider", p.Name(), "model", c.ID)
			continue
		}
		attemptsStarted++

		attemptBody, err := setModel(body, c.ID)
		if err != nil {
			last = ChatResult{Status: 400, Provider: c.Provider, Model: c.ID, Attempted: last.Attempted}
			lastErr = err
			logging.Debugf(ctx, "route_attempt_prepare_failed", "provider", c.Provider, "model", c.ID, "err", err.Error())
			continue
		}
		attemptBody, err = SanitizeChatBody(p.Name(), attemptBody)
		if err != nil {
			last = ChatResult{Status: 400, Provider: c.Provider, Model: c.ID, Attempted: last.Attempted}
			lastErr = err
			logging.Debugf(ctx, "route_attempt_sanitize_failed", "provider", p.Name(), "model", c.ID, "err", err.Error())
			continue
		}

		attemptStart := time.Now()
		actx, cancel := context.WithTimeout(ctx, r.opts.UpstreamTimeout)
		resp, status, hdr, err := p.ChatCompletions(actx, attemptBody)
		timedOut := errors.Is(actx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded)
		cancel()
		if timedOut {
			status = 504
			if err == nil || errors.Is(err, context.DeadlineExceeded) {
				err = fmt.Errorf("upstream timeout after %s", r.opts.UpstreamTimeout)
			}
		}
		r.persistRateLimits(ctx, p.Name(), c.ID, hdr)
		attemptMS := time.Since(attemptStart).Milliseconds()

		tried := append(last.Tried, p.Name()+"/"+c.ID)
		rec := AttemptRecord{
			Provider:  p.Name(),
			Model:     c.ID,
			Status:    status,
			LatencyMS: attemptMS,
			Body:      resp,
		}
		if err != nil || status < 200 || status >= 300 {
			rec.ErrorType = classifyAttemptError(status, err)
		}
		attempted := append(last.Attempted, rec)

		result := ChatResult{
			Body:      resp,
			Status:    status,
			Headers:   hdr,
			Provider:  p.Name(),
			Model:     c.ID,
			Attempts:  len(tried),
			Tried:     tried,
			Attempted: attempted,
		}
		last = result
		lastErr = err

		errStr := ""
		if err != nil {
			errStr = err.Error()
		}
		logging.Debugf(ctx, "route_attempt",
			"n", i+1,
			"provider", p.Name(),
			"model", c.ID,
			"status", status,
			"latency_ms", attemptMS,
			"upstream_id", peekUpstreamID(resp),
			"err", errStr,
		)

		if err == nil && status >= 200 && status < 300 {
			if IsEmptyLengthCompletion(resp) {
				logging.Infof(ctx, "empty_length_completion", "provider", p.Name(), "model", c.ID)
				if _, did := emptyRetried[c.ID]; !did {
					emptyRetried[c.ID] = struct{}{}
					if bumped, berr := BumpMaxTokens(body, 256); berr == nil {
						body = bumped
					}
					// One immediate retry on the same model with a safer budget.
					retryBody, berr := setModel(body, c.ID)
					if berr == nil {
						retryBody, berr = SanitizeChatBody(p.Name(), retryBody)
					}
					if berr == nil {
						rctx, rcancel := context.WithTimeout(ctx, r.opts.UpstreamTimeout)
						resp2, status2, hdr2, err2 := p.ChatCompletions(rctx, retryBody)
						rcancel()
						r.persistRateLimits(ctx, p.Name(), c.ID, hdr2)
						attemptsStarted++
						tried = append(tried, p.Name()+"/"+c.ID)
						rec2 := AttemptRecord{Provider: p.Name(), Model: c.ID, Status: status2, Body: resp2}
						if err2 != nil || status2 < 200 || status2 >= 300 {
							rec2.ErrorType = classifyAttemptError(status2, err2)
						}
						attempted = append(attempted, rec2)
						result.Attempts = len(tried)
						result.Tried = tried
						result.Attempted = attempted
						if err2 == nil && status2 >= 200 && status2 < 300 && !IsEmptyLengthCompletion(resp2) {
							result.Body = resp2
							result.Status = status2
							result.Headers = hdr2
							return result, nil
						}
						last = result
					}
				}
				lastErr = fmt.Errorf("empty completion (token budget exhausted) from %s/%s", p.Name(), c.ID)
				logging.Infof(ctx, "route_fallback", "from", p.Name()+"/"+c.ID, "status", status, "reason", "empty_length")
				continue
			}
			return result, nil
		}

		// Soft-deny models that are listed but cannot serve chat right now.
		if isModelUnusable(status, resp, err) {
			r.denyModel(c.ID, r.denyTTL())
			logging.Infof(ctx, "model_soft_denied", "provider", p.Name(), "model", c.ID, "status", status)
		}

		retryable := isRetryableStatus(status) || isModelUnusable(status, resp, err) || isUnsupportedParamError(resp)
		if !retryable {
			logging.Infof(ctx, "route_stop_non_retryable", "provider", p.Name(), "model", c.ID, "status", status)
			return result, err
		}
		if status == 429 || status >= 500 {
			failedProviders[p.Name()] = struct{}{}
		}
		logging.Infof(ctx, "route_fallback", "from", p.Name()+"/"+c.ID, "status", status, "attempt", attemptsStarted)
	}

	if last.Status == 0 {
		last.Status = 503
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("all routing candidates failed")
	}
	return last, lastErr
}

func classifyAttemptError(status int, err error) string {
	if status == 429 {
		return "rate_limit"
	}
	if status == 504 || (err != nil && strings.Contains(err.Error(), "timeout")) {
		return "timeout"
	}
	if status == 402 {
		return "credits"
	}
	if status == 404 {
		return "not_found"
	}
	if status == 400 {
		return "bad_request"
	}
	if err != nil || status >= 400 {
		return "upstream"
	}
	return ""
}

func (r *Registry) denyTTL() time.Duration {
	ttl := 10 * time.Minute
	if r.opts.ModelsCacheTTL > 0 {
		if d := r.opts.ModelsCacheTTL * 4; d > ttl {
			ttl = d
		}
	}
	return ttl
}

func (r *Registry) denyModel(modelID string, ttl time.Duration) {
	if modelID == "" || ttl <= 0 {
		return
	}
	r.denyMu.Lock()
	defer r.denyMu.Unlock()
	if r.denyUntil == nil {
		r.denyUntil = make(map[string]time.Time)
	}
	r.denyUntil[modelID] = time.Now().Add(ttl)
}

func (r *Registry) isDenied(modelID string) bool {
	r.denyMu.Lock()
	defer r.denyMu.Unlock()
	until, ok := r.denyUntil[modelID]
	if !ok {
		return false
	}
	if time.Now().After(until) {
		delete(r.denyUntil, modelID)
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

func isModelUnusable(status int, body json.RawMessage, err error) bool {
	if status == 404 {
		return true
	}
	msg := strings.ToLower(string(body))
	if err != nil {
		msg += " " + strings.ToLower(err.Error())
	}
	needles := []string{
		"model not found",
		"is not found",
		"not found for",
		"does not exist",
		"unknown model",
		"invalid model",
		"unsupported model",
		"no longer available",
		"model is not available",
	}
	for _, n := range needles {
		if strings.Contains(msg, n) {
			return true
		}
	}
	return false
}

func isUnsupportedParamError(body json.RawMessage) bool {
	msg := strings.ToLower(string(body))
	return strings.Contains(msg, "unknown name") ||
		strings.Contains(msg, "cannot find field") ||
		strings.Contains(msg, "unsupported parameter") ||
		strings.Contains(msg, "extra fields not permitted") ||
		strings.Contains(msg, "unrecognized request argument")
}

func (r *Registry) candidates(ctx context.Context, model string, auto bool, opts RouteOptions) ([]Model, string, error) {
	filter := ListFilter{
		Provider:    opts.Provider,
		Recommended: opts.RecommendedOnly,
	}
	listed, err := r.ListModels(ctx, filter)
	if err != nil && len(listed.Data) == 0 {
		return nil, "", err
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
		if r.isDenied(m.ID) {
			logging.Debugf(ctx, "candidate_skip_denied", "provider", m.Provider, "model", m.ID)
			continue
		}
		if r.providerOpenCircuit(ctx, m.Provider) {
			logging.Debugf(ctx, "candidate_skip_circuit", "provider", m.Provider, "model", m.ID)
			continue
		}
		if !isRoutable(m) {
			logging.Debugf(ctx, "candidate_skip_quota", "provider", m.Provider, "model", m.ID)
			continue
		}
		available = append(available, m)
	}

	if auto {
		return available, "none", nil
	}

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
	if primary != nil {
		out := make([]Model, 0, 1+len(rest))
		out = append(out, *primary)
		out = append(out, rest...)
		return out, "", nil
	}

	// Model known but skipped (quota/circuit): still force one try for explicit id.
	// Other routable models remain as fallbacks.
	known, providerName := r.lookupModelProvider(ctx, model, listed.Data)
	if !known {
		return nil, "not_found", nil
	}
	forced := Model{
		ID:       model,
		Provider: providerName,
		Object:   "model",
		OwnedBy:  providerName,
	}
	out := make([]Model, 0, 1+len(rest))
	out = append(out, forced)
	out = append(out, rest...)
	return out, "", nil
}

func (r *Registry) lookupModelProvider(ctx context.Context, model string, listed []Model) (bool, string) {
	for _, m := range listed {
		if m.ID == model {
			return true, m.Provider
		}
	}
	r.mu.RLock()
	p, ok := r.byModel[model]
	r.mu.RUnlock()
	if !ok {
		_, _ = r.cachedModels(ctx)
		r.mu.RLock()
		p, ok = r.byModel[model]
		r.mu.RUnlock()
	}
	if !ok {
		return false, ""
	}
	return true, p.Name()
}

func (r *Registry) providerOpenCircuit(ctx context.Context, providerName string) bool {
	if r.store == nil || r.opts.CircuitErrors <= 0 {
		return false
	}
	h, ok, err := r.store.GetProviderHealth(ctx, providerName)
	if err != nil || !ok {
		return false
	}
	if h.ConsecutiveErrors < r.opts.CircuitErrors {
		return false
	}
	if h.LastErrorAt == nil {
		return true
	}
	return time.Since(*h.LastErrorAt) < r.opts.CircuitCooldown
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
	if m.Quota.RemainingTPM != nil && *m.Quota.RemainingTPM <= 0 {
		return false
	}
	return true
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

func peekUpstreamID(body json.RawMessage) string {
	if len(body) == 0 {
		return ""
	}
	var parsed struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return ""
	}
	return parsed.ID
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
