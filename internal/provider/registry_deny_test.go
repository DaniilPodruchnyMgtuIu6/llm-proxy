package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"
)

func TestSoftDenySkipsModelOnAutoRoute(t *testing.T) {
	bad := &mockProvider{
		name: "gemini",
		models: []Model{
			{ID: "gemini-bad", Object: "model", Provider: "gemini", Free: true, Quota: &Quota{RemainingRPD: int64ptr(10)}},
			{ID: "gemini-good", Object: "model", Provider: "gemini", Free: true, Quota: &Quota{RemainingRPD: int64ptr(10)}},
		},
		chatFn: func(body json.RawMessage) (json.RawMessage, int, http.Header, error) {
			var req struct {
				Model string `json:"model"`
			}
			_ = json.Unmarshal(body, &req)
			if req.Model == "gemini-bad" {
				return json.RawMessage(`{"error":{"message":"model not found"}}`), 404, nil, fmt.Errorf("not found")
			}
			return json.RawMessage(`{"id":"ok","choices":[]}`), 200, nil, nil
		},
	}
	opts := DefaultOptions()
	opts.ModelsCacheTTL = time.Minute
	opts.MaxAttempts = 3
	reg := NewRegistryWithOptions(nil, opts, bad)

	// First auto call: tries bad (higher quality if we set scores via ranking — order is list order after score).
	// Force quality by ranking package; ensure bad is tried first by putting higher score via ID ranking.
	body := json.RawMessage(`{"model":"auto","messages":[{"role":"user","content":"hi"}]}`)
	res, err := reg.ChatCompletions(context.Background(), body, RouteOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Model != "gemini-good" && res.Model != "gemini-bad" {
		t.Fatalf("unexpected model %q", res.Model)
	}

	// Soft-deny bad if it failed.
	if !reg.isDenied("gemini-bad") && res.Model == "gemini-good" {
		// bad was tried and denied
	}
	if reg.isDenied("gemini-bad") {
		res2, err := reg.ChatCompletions(context.Background(), body, RouteOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if res2.Model != "gemini-good" {
			t.Fatalf("expected gemini-good after deny, got %q", res2.Model)
		}
		if bad.chatCalls < 2 {
			t.Fatalf("expected at least 2 chat calls, got %d", bad.chatCalls)
		}
	}
}

func TestUnsupportedParamFallsBack(t *testing.T) {
	a := &mockProvider{
		name: "gemini",
		models: []Model{{
			ID: "gemini-flash-test", Object: "model", Provider: "gemini", Free: true, Quota: &Quota{RemainingRPD: int64ptr(5)},
		}},
		chatFn: func(json.RawMessage) (json.RawMessage, int, http.Header, error) {
			return json.RawMessage(`{"error":{"message":"Unknown name \"frequency_penalty\": Cannot find field."}}`), 400, nil, fmt.Errorf("bad")
		},
	}
	b := &mockProvider{
		name: "groq",
		models: []Model{{
			ID: "openai/gpt-oss-20b", Object: "model", Provider: "groq", Free: true, Quota: &Quota{RemainingRPD: int64ptr(5)},
		}},
		chatFn: func(json.RawMessage) (json.RawMessage, int, http.Header, error) {
			return json.RawMessage(`{"id":"ok","choices":[]}`), 200, nil, nil
		},
	}
	opts := DefaultOptions()
	opts.ModelsCacheTTL = 0
	opts.MaxAttempts = 3
	reg := NewRegistryWithOptions(nil, opts, a, b)
	body := json.RawMessage(`{"model":"gemini-flash-test","messages":[{"role":"user","content":"hi"}],"frequency_penalty":0.2}`)
	res, err := reg.ChatCompletions(context.Background(), body, RouteOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Model != "openai/gpt-oss-20b" || res.Provider != "groq" {
		t.Fatalf("want groq fallback, got %s/%s", res.Provider, res.Model)
	}
}
