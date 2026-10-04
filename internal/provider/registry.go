package provider

import (
	"context"
	"encoding/json"
	"fmt"
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

func (r *Registry) ChatCompletions(ctx context.Context, body json.RawMessage) (json.RawMessage, int, string, string, error) {
	var req struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, 400, "", "", fmt.Errorf("invalid request body: %w", err)
	}
	if strings.TrimSpace(req.Model) == "" {
		return nil, 400, "", "", fmt.Errorf("model is required")
	}

	r.mu.RLock()
	p, ok := r.byModel[req.Model]
	r.mu.RUnlock()

	if !ok {
		if _, err := r.ListModels(ctx, ListFilter{}); err != nil {
			return nil, 502, "", "", err
		}
		r.mu.RLock()
		p, ok = r.byModel[req.Model]
		r.mu.RUnlock()
	}
	if !ok {
		return nil, 404, "", "", fmt.Errorf("model not found: %s", req.Model)
	}

	resp, status, err := p.ChatCompletions(ctx, body)
	return resp, status, p.Name(), req.Model, err
}

// ResolvePoolID best-effort: openrouter free pool, else empty.
func ResolvePoolID(providerName, modelID string) string {
	if providerName == "openrouter" {
		return "openrouter:free"
	}
	return ""
}
