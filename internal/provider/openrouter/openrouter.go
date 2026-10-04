package openrouter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/llm-proxy/llm-proxy/internal/provider"
	"github.com/llm-proxy/llm-proxy/internal/quota"
)

const providerName = "openrouter"

// Client talks to OpenRouter via the OpenAI-compatible API.
// Docs: https://openrouter.ai/docs/quickstart
type Client struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
	siteURL    string
	siteTitle  string
}

func New(apiKey, baseURL, siteURL, siteTitle string) *Client {
	return &Client{
		apiKey:    apiKey,
		baseURL:   strings.TrimRight(baseURL, "/"),
		siteURL:   siteURL,
		siteTitle: siteTitle,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

func (c *Client) Name() string        { return providerName }
func (c *Client) DisplayName() string { return "OpenRouter" }
func (c *Client) DocsURL() string     { return "https://openrouter.ai/docs/quickstart" }

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
		return nil, fmt.Errorf("openrouter models: status %d: %s", resp.StatusCode, truncate(body, 512))
	}

	var raw struct {
		Data []struct {
			ID      string `json:"id"`
			Name    string `json:"name"`
			Created int64  `json:"created"`
			Pricing *struct {
				Prompt     string `json:"prompt"`
				Completion string `json:"completion"`
			} `json:"pricing"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("decode models: %w", err)
	}

	poolQuota := c.freePoolQuota(ctx)
	src := provider.Source{ID: providerName, Name: c.DisplayName()}
	out := make([]provider.Model, 0)
	for _, m := range raw.Data {
		if !isFreeModel(m.ID, m.Pricing != nil && isZeroPrice(m.Pricing.Prompt) && isZeroPrice(m.Pricing.Completion)) {
			continue
		}
		out = append(out, provider.Model{
			ID:          m.ID,
			Object:      "model",
			Created:     m.Created,
			OwnedBy:     ownerFromID(m.ID),
			Provider:    providerName,
			Source:      src,
			Free:        true,
			Recommended: quota.IsOpenRouterRecommended(m.ID),
			Quota:       quota.Clone(poolQuota),
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
		Quota:   c.freePoolQuota(ctx),
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

func (c *Client) freePoolQuota(ctx context.Context) *provider.Quota {
	base := quota.OpenRouterFreePoolBase()
	remaining, used, limit, ok := c.fetchFreeDaily(ctx)
	if !ok {
		return base
	}
	return quota.ApplyLiveRemaining(base, remaining, used, limit)
}

func (c *Client) fetchFreeDaily(ctx context.Context) (remaining, used, limit *int64, ok bool) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/key", nil)
	if err != nil {
		return nil, nil, nil, false
	}
	c.setAuth(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, nil, nil, false
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil || resp.StatusCode >= 300 {
		return nil, nil, nil, false
	}

	var raw struct {
		Data struct {
			FreeModelDailyRequests *struct {
				Limit     int64 `json:"limit"`
				Remaining int64 `json:"remaining"`
				Used      int64 `json:"used"`
			} `json:"free_model_daily_requests"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &raw); err != nil || raw.Data.FreeModelDailyRequests == nil {
		return nil, nil, nil, false
	}
	f := raw.Data.FreeModelDailyRequests
	return &f.Remaining, &f.Used, &f.Limit, true
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
		return respBody, resp.StatusCode, fmt.Errorf("openrouter chat: status %d", resp.StatusCode)
	}
	return json.RawMessage(respBody), resp.StatusCode, nil
}

func (c *Client) setAuth(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	if c.siteURL != "" {
		req.Header.Set("HTTP-Referer", c.siteURL)
	}
	if c.siteTitle != "" {
		req.Header.Set("X-OpenRouter-Title", c.siteTitle)
	}
}

func isFreeModel(id string, zeroPrice bool) bool {
	if id == "openrouter/free" {
		return true
	}
	if strings.HasSuffix(id, ":free") {
		return true
	}
	return zeroPrice
}

func isZeroPrice(v string) bool {
	v = strings.TrimSpace(v)
	if v == "" {
		return false
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return v == "0" || v == "0.0"
	}
	return f == 0
}

func ownerFromID(id string) string {
	id = strings.TrimSuffix(id, ":free")
	if i := strings.IndexByte(id, '/'); i > 0 {
		return id[:i]
	}
	return "openrouter"
}

func truncate(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "..."
}
