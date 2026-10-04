package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"
)

func TestMaxFallbackAttempts(t *testing.T) {
	a := &mockProvider{
		name: "a",
		models: []Model{{
			ID: "a-model", Object: "model", Provider: "a", Free: true,
			Quota: &Quota{RemainingRPD: int64ptr(10)},
		}},
		chatFn: func(json.RawMessage) (json.RawMessage, int, http.Header, error) {
			return json.RawMessage(`{"error":"rl"}`), 429, nil, fmt.Errorf("rl")
		},
	}
	b := &mockProvider{
		name: "b",
		models: []Model{{
			ID: "b-model", Object: "model", Provider: "b", Free: true,
			Quota: &Quota{RemainingRPD: int64ptr(10)},
		}},
		chatFn: func(json.RawMessage) (json.RawMessage, int, http.Header, error) {
			return json.RawMessage(`{"error":"rl"}`), 429, nil, fmt.Errorf("rl")
		},
	}
	c := &mockProvider{
		name: "c",
		models: []Model{{
			ID: "c-model", Object: "model", Provider: "c", Free: true,
			Quota: &Quota{RemainingRPD: int64ptr(10)},
		}},
		chatFn: func(json.RawMessage) (json.RawMessage, int, http.Header, error) {
			return json.RawMessage(`{"id":"ok"}`), 200, nil, nil
		},
	}
	opts := DefaultOptions()
	opts.MaxAttempts = 2
	opts.ModelsCacheTTL = 0
	reg := NewRegistryWithOptions(nil, opts, a, b, c)
	body := json.RawMessage(`{"model":"auto","messages":[{"role":"user","content":"hi"}]}`)
	res, err := reg.ChatCompletions(context.Background(), body, RouteOptions{})
	if err == nil {
		t.Fatal("expected failure after max attempts")
	}
	if res.Attempts != 2 {
		t.Fatalf("attempts=%d", res.Attempts)
	}
	if c.chatCalls != 0 {
		t.Fatal("third provider should not be called when max=2")
	}
	if len(res.Attempted) != 2 {
		t.Fatalf("attempted=%d", len(res.Attempted))
	}
}

func TestSkipSiblingModelsAfterProvider429(t *testing.T) {
	gem := &mockProvider{
		name: "gemini",
		models: []Model{
			{ID: "gemini-pro", Object: "model", Provider: "gemini", Free: true, Quota: &Quota{RemainingRPD: int64ptr(10)}},
			{ID: "gemini-flash", Object: "model", Provider: "gemini", Free: true, Quota: &Quota{RemainingRPD: int64ptr(10)}},
		},
		chatFn: func(json.RawMessage) (json.RawMessage, int, http.Header, error) {
			return json.RawMessage(`{"error":"rl"}`), 429, nil, fmt.Errorf("rl")
		},
	}
	or := &mockProvider{
		name: "openrouter",
		models: []Model{{
			ID: "openrouter/free", Object: "model", Provider: "openrouter", Free: true,
			Quota: &Quota{RemainingRPD: int64ptr(10)},
		}},
		chatFn: func(json.RawMessage) (json.RawMessage, int, http.Header, error) {
			return json.RawMessage(`{"id":"ok"}`), 200, nil, nil
		},
	}
	opts := DefaultOptions()
	opts.MaxAttempts = 3
	opts.ModelsCacheTTL = 0
	reg := NewRegistryWithOptions(nil, opts, gem, or)
	body := json.RawMessage(`{"model":"auto","messages":[{"role":"user","content":"hi"}]}`)
	res, err := reg.ChatCompletions(context.Background(), body, RouteOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Model != "openrouter/free" {
		t.Fatalf("got %s", res.Model)
	}
	if gem.chatCalls != 1 {
		t.Fatalf("gemini should be tried once, got %d", gem.chatCalls)
	}
}

func TestSkipZeroTPM(t *testing.T) {
	exhausted := &mockProvider{
		name: "groq",
		models: []Model{{
			ID: "openai/gpt-oss-20b", Object: "model", Provider: "groq", Free: true,
			Quota: &Quota{RemainingTPM: int64ptr(0), RemainingRPD: int64ptr(10)},
		}},
	}
	ok := &mockProvider{
		name: "openrouter",
		models: []Model{{
			ID: "openrouter/free", Object: "model", Provider: "openrouter", Free: true,
			Quota: &Quota{RemainingRPD: int64ptr(5)},
		}},
	}
	opts := DefaultOptions()
	opts.ModelsCacheTTL = 0
	reg := NewRegistryWithOptions(nil, opts, exhausted, ok)
	body := json.RawMessage(`{"messages":[{"role":"user","content":"hi"}]}`)
	res, err := reg.ChatCompletions(context.Background(), body, RouteOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Model != "openrouter/free" {
		t.Fatalf("got %s", res.Model)
	}
	if exhausted.chatCalls != 0 {
		t.Fatal("zero TPM model should be skipped")
	}
}

type timeoutProvider struct {
	calls int
}

func (p *timeoutProvider) Name() string        { return "slow" }
func (p *timeoutProvider) DisplayName() string { return "slow" }
func (p *timeoutProvider) DocsURL() string     { return "http://example.test" }
func (p *timeoutProvider) ListModels(context.Context) ([]Model, error) {
	return []Model{{
		ID: "slow-model", Object: "model", OwnedBy: "slow", Provider: "slow", Free: true,
		Quota: &Quota{RemainingRPD: int64ptr(1)},
	}}, nil
}
func (p *timeoutProvider) Status(context.Context) (Status, error) {
	return Status{ID: "slow", Name: "slow", Enabled: true, Healthy: true, Models: 1}, nil
}
func (p *timeoutProvider) ChatCompletions(ctx context.Context, _ json.RawMessage) (json.RawMessage, int, http.Header, error) {
	p.calls++
	<-ctx.Done()
	return nil, 0, nil, ctx.Err()
}

func TestUpstreamTimeout(t *testing.T) {
	slow := &timeoutProvider{}
	opts := DefaultOptions()
	opts.UpstreamTimeout = 50 * time.Millisecond
	opts.MaxAttempts = 1
	opts.ModelsCacheTTL = 0
	reg := NewRegistryWithOptions(nil, opts, slow)
	body := json.RawMessage(`{"model":"slow-model","messages":[{"role":"user","content":"hi"}]}`)
	res, err := reg.ChatCompletions(context.Background(), body, RouteOptions{})
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if res.Status != 504 {
		t.Fatalf("status=%d err=%v", res.Status, err)
	}
	if slow.calls != 1 {
		t.Fatalf("calls=%d", slow.calls)
	}
}
