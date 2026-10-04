package provider

import (
	"context"
	"encoding/json"
)

// Source identifies the backend that serves a model.
type Source struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Quota describes request/token budgets for routing decisions.
// Null numeric fields mean "unknown / not tracked".
type Quota struct {
	RPM *int64 `json:"rpm"`
	RPD *int64 `json:"rpd"`
	TPM *int64 `json:"tpm,omitempty"`
	TPD *int64 `json:"tpd,omitempty"`

	RemainingRPM *int64 `json:"remaining_rpm"`
	RemainingRPD *int64 `json:"remaining_rpd"`
	UsedRPD      *int64 `json:"used_rpd,omitempty"`

	// reset_rpd_hint: midnight_utc | midnight_pacific | sliding_window | unknown
	ResetRPDHint string `json:"reset_rpd_hint,omitempty"`
	// resets_at is next RPD reset in RFC3339 when computable.
	ResetsAt string `json:"resets_at,omitempty"`

	// scope: per_model | shared_pool
	Scope  string `json:"scope"`
	PoolID string `json:"pool_id,omitempty"`

	// source: live | static_catalog | unknown
	Source string `json:"source"`
	// confidence: exact | estimate | unknown
	Confidence string `json:"confidence"`
	Tier       string `json:"tier,omitempty"`
	Notes      string `json:"notes,omitempty"`
}

// Model is an OpenAI-compatible model entry enriched for the main system.
type Model struct {
	ID          string  `json:"id"`
	Object      string  `json:"object"`
	Created     int64   `json:"created,omitempty"`
	OwnedBy     string  `json:"owned_by"`
	Provider    string  `json:"provider"`
	Source      Source  `json:"source"`
	Free        bool    `json:"free"`
	Recommended bool    `json:"recommended,omitempty"`
	Quota       *Quota  `json:"quota"`
}

// ModelsResponse is returned by GET /v1/models.
type ModelsResponse struct {
	Object string  `json:"object"`
	Data   []Model `json:"data"`
}

// Status is a provider-level snapshot for GET /v1/providers.
type Status struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Enabled     bool   `json:"enabled"`
	Healthy     bool   `json:"healthy"`
	Error       string `json:"error,omitempty"`
	Models      int    `json:"models"`
	FreeModels  int    `json:"free_models"`
	Quota       *Quota `json:"quota,omitempty"`
	Docs        string `json:"docs,omitempty"`
}

// ProvidersResponse is returned by GET /v1/providers.
type ProvidersResponse struct {
	Object string   `json:"object"`
	Data   []Status `json:"data"`
}

// ListFilter controls GET /v1/models query options.
type ListFilter struct {
	Provider    string
	FreeOnly    bool
	Recommended bool
}

// Provider is a free/paid LLM backend behind the proxy.
type Provider interface {
	Name() string
	DisplayName() string
	DocsURL() string
	ListModels(ctx context.Context) ([]Model, error)
	Status(ctx context.Context) (Status, error)
	ChatCompletions(ctx context.Context, body json.RawMessage) (json.RawMessage, int, error)
}
