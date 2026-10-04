package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
)

type mockProvider struct {
	name     string
	models   []Model
	chatFn   func(body json.RawMessage) (json.RawMessage, int, http.Header, error)
	chatCalls int
}

func (m *mockProvider) Name() string        { return m.name }
func (m *mockProvider) DisplayName() string { return m.name }
func (m *mockProvider) DocsURL() string     { return "http://example.test" }
func (m *mockProvider) ListModels(context.Context) ([]Model, error) {
	out := make([]Model, len(m.models))
	copy(out, m.models)
	return out, nil
}
func (m *mockProvider) Status(context.Context) (Status, error) {
	return Status{ID: m.name, Name: m.name, Enabled: true, Healthy: true, Models: len(m.models)}, nil
}
func (m *mockProvider) ChatCompletions(_ context.Context, body json.RawMessage) (json.RawMessage, int, http.Header, error) {
	m.chatCalls++
	if m.chatFn != nil {
		return m.chatFn(body)
	}
	return json.RawMessage(`{"id":"ok","choices":[]}`), 200, http.Header{}, nil
}

func TestChatCompletionsFallbackOn429(t *testing.T) {
	a := &mockProvider{
		name: "alpha",
		models: []Model{{
			ID: "alpha-model", Object: "model", OwnedBy: "alpha", Provider: "alpha", Free: true,
			Quota: &Quota{RPD: int64ptr(100), RemainingRPD: int64ptr(50), RemainingRPM: int64ptr(10), Scope: "per_model", Source: "static_catalog"},
		}},
		chatFn: func(json.RawMessage) (json.RawMessage, int, http.Header, error) {
			return json.RawMessage(`{"error":"rate"}`), 429, http.Header{"X-Ratelimit-Remaining-Requests": []string{"0"}}, fmt.Errorf("rate")
		},
	}
	b := &mockProvider{
		name: "beta",
		models: []Model{{
			ID: "beta-model", Object: "model", OwnedBy: "beta", Provider: "beta", Free: true, Recommended: true,
			Quota: &Quota{RPD: int64ptr(100), RemainingRPD: int64ptr(50), RemainingRPM: int64ptr(10), Scope: "per_model", Source: "static_catalog"},
		}},
		chatFn: func(body json.RawMessage) (json.RawMessage, int, http.Header, error) {
			var req struct {
				Model string `json:"model"`
			}
			_ = json.Unmarshal(body, &req)
			if req.Model != "beta-model" {
				t.Fatalf("expected beta-model, got %q", req.Model)
			}
			h := http.Header{}
			h.Set("x-ratelimit-remaining-requests", "9")
			return json.RawMessage(`{"id":"beta-ok","choices":[]}`), 200, h, nil
		},
	}

	reg := NewRegistry(nil, a, b)
	body := json.RawMessage(`{"model":"alpha-model","messages":[{"role":"user","content":"hi"}]}`)
	res, err := reg.ChatCompletions(context.Background(), body, RouteOptions{})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if res.Model != "beta-model" || res.Provider != "beta" {
		t.Fatalf("got %s/%s", res.Provider, res.Model)
	}
	if res.Attempts != 2 {
		t.Fatalf("attempts=%d", res.Attempts)
	}
	if a.chatCalls != 1 || b.chatCalls != 1 {
		t.Fatalf("calls a=%d b=%d", a.chatCalls, b.chatCalls)
	}
}

func TestChatCompletionsAutoSkipsExhausted(t *testing.T) {
	exhausted := &mockProvider{
		name: "gemini",
		models: []Model{{
			ID: "gemini-2.5-pro", Object: "model", OwnedBy: "google", Provider: "gemini", Free: true,
			Quota: &Quota{RemainingRPD: int64ptr(0), Scope: "per_model"},
		}},
	}
	ok := &mockProvider{
		name: "groq",
		models: []Model{{
			ID: "openai/gpt-oss-20b", Object: "model", OwnedBy: "openai", Provider: "groq", Free: true,
			Quota: &Quota{RemainingRPD: int64ptr(5), Scope: "per_model"},
		}},
	}

	reg := NewRegistry(nil, exhausted, ok)
	body := json.RawMessage(`{"model":"auto","messages":[{"role":"user","content":"hi"}]}`)
	res, err := reg.ChatCompletions(context.Background(), body, RouteOptions{})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if res.Model != "openai/gpt-oss-20b" {
		t.Fatalf("expected available model, got %s", res.Model)
	}
	if exhausted.chatCalls != 0 {
		t.Fatalf("exhausted provider should not be called")
	}
}

func TestChatCompletionsNoFallbackOn400(t *testing.T) {
	a := &mockProvider{
		name: "alpha",
		models: []Model{{
			ID: "alpha-model", Object: "model", OwnedBy: "alpha", Provider: "alpha", Free: true,
			Quota: &Quota{RemainingRPD: int64ptr(10)},
		}},
		chatFn: func(json.RawMessage) (json.RawMessage, int, http.Header, error) {
			return json.RawMessage(`{"error":"bad"}`), 400, nil, fmt.Errorf("bad request")
		},
	}
	b := &mockProvider{
		name: "beta",
		models: []Model{{
			ID: "beta-model", Object: "model", OwnedBy: "beta", Provider: "beta", Free: true,
			Quota: &Quota{RemainingRPD: int64ptr(10)},
		}},
	}
	reg := NewRegistry(nil, a, b)
	body := json.RawMessage(`{"model":"alpha-model","messages":[{"role":"user","content":"hi"}]}`)
	res, err := reg.ChatCompletions(context.Background(), body, RouteOptions{})
	if err == nil {
		t.Fatal("expected error")
	}
	if res.Status != 400 || res.Model != "alpha-model" {
		t.Fatalf("status=%d model=%s", res.Status, res.Model)
	}
	if b.chatCalls != 0 {
		t.Fatalf("should not fallback, beta calls=%d", b.chatCalls)
	}
}

func int64ptr(v int64) *int64 { return &v }
