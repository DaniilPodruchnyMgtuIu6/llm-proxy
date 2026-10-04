package gemini

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/llm-proxy/llm-proxy/internal/provider"
	"github.com/llm-proxy/llm-proxy/internal/quota"
)

const providerName = "gemini"

// Client talks to Gemini via the OpenAI-compatible API.
// Docs: https://ai.google.dev/gemini-api/docs/openai
type Client struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

func New(apiKey, baseURL string) *Client {
	return &Client{
		apiKey:  apiKey,
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

func (c *Client) Name() string        { return providerName }
func (c *Client) DisplayName() string { return "Google Gemini (AI Studio)" }
func (c *Client) DocsURL() string     { return "https://aistudio.google.com/docs" }

func (c *Client) ListModels(ctx context.Context) ([]provider.Model, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/models", nil)
	if err != nil {
		return nil, err
	}
	c.setAuth(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("gemini models: status %d: %s", resp.StatusCode, truncate(body, 512))
	}

	var raw struct {
		Data []struct {
			ID      string `json:"id"`
			Object  string `json:"object"`
			Created int64  `json:"created"`
			OwnedBy string `json:"owned_by"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("decode models: %w", err)
	}

	src := provider.Source{ID: providerName, Name: c.DisplayName()}
	out := make([]provider.Model, 0, len(raw.Data))
	for _, m := range raw.Data {
		id := strings.TrimPrefix(m.ID, "models/")
		owned := m.OwnedBy
		if owned == "" {
			owned = "google"
		}
		obj := m.Object
		if obj == "" {
			obj = "model"
		}
		out = append(out, provider.Model{
			ID:          id,
			Object:      obj,
			Created:     m.Created,
			OwnedBy:     owned,
			Provider:    providerName,
			Source:      src,
			Free:        true,
			Recommended: quota.IsGeminiRecommended(id),
			Quota:       quota.GeminiQuota(id),
		})
	}
	return out, nil
}

func (c *Client) Status(ctx context.Context) (provider.Status, error) {
	models, err := c.ListModels(ctx)
	st := provider.Status{
		ID:      providerName,
		Name:    c.DisplayName(),
		Enabled: true,
		Docs:    c.DocsURL(),
		Quota: &provider.Quota{
			Scope:        "per_model",
			Source:       "static_catalog",
			Confidence:   "estimate",
			Tier:         "free",
			ResetRPDHint: "midnight_pacific",
			ResetsAt:     quota.NextMidnightPacific(time.Now()),
			Notes:        "Per-model free-tier limits; remaining not available via API.",
		},
	}
	if err != nil {
		st.Healthy = false
		st.Error = err.Error()
		return st, nil
	}
	st.Healthy = true
	st.Models = len(models)
	st.FreeModels = len(models)
	return st, nil
}

func (c *Client) ChatCompletions(ctx context.Context, body json.RawMessage) (json.RawMessage, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, 500, err
	}
	c.setAuth(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, 502, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 502, err
	}
	if resp.StatusCode >= 300 {
		return respBody, resp.StatusCode, fmt.Errorf("gemini chat: status %d", resp.StatusCode)
	}
	return json.RawMessage(respBody), resp.StatusCode, nil
}

func (c *Client) setAuth(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
}

func truncate(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "..."
}
