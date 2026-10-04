package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

func TestSanitizeChatBodyDropsUnsupported(t *testing.T) {
	body := json.RawMessage(`{
		"model":"x",
		"messages":[{"role":"user","content":"hi","name":"bob"}],
		"temperature":0.7,
		"top_k":40,
		"logprobs":true,
		"max_tokens":128,
		"unknown_field":1
	}`)

	got, err := SanitizeChatBody("groq", body)
	if err != nil {
		t.Fatal(err)
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(got, &obj); err != nil {
		t.Fatal(err)
	}
	if _, ok := obj["temperature"]; !ok {
		t.Fatal("temperature should pass")
	}
	if _, ok := obj["max_tokens"]; !ok {
		t.Fatal("max_tokens should pass")
	}
	if _, ok := obj["top_k"]; ok {
		t.Fatal("top_k should be dropped for groq")
	}
	if _, ok := obj["logprobs"]; ok {
		t.Fatal("logprobs should be dropped for groq")
	}
	if _, ok := obj["unknown_field"]; ok {
		t.Fatal("unknown should be dropped")
	}
	var msgs []map[string]any
	if err := json.Unmarshal(obj["messages"], &msgs); err != nil {
		t.Fatal(err)
	}
	if _, has := msgs[0]["name"]; has {
		t.Fatal("messages[].name should be stripped for groq")
	}
}

func TestSanitizeChatBodyOpenRouterKeepsTopK(t *testing.T) {
	body := json.RawMessage(`{"model":"m","messages":[{"role":"user","content":"hi"}],"top_k":20,"temperature":0.2}`)
	got, err := SanitizeChatBody("openrouter", body)
	if err != nil {
		t.Fatal(err)
	}
	var obj map[string]any
	if err := json.Unmarshal(got, &obj); err != nil {
		t.Fatal(err)
	}
	if obj["top_k"] == nil {
		t.Fatal("expected top_k for openrouter")
	}
}

func TestSanitizeChatBodyGeminiDropsPenalties(t *testing.T) {
	body := json.RawMessage(`{
		"model":"gemini-3.5-flash-lite",
		"messages":[{"role":"user","content":"hi"}],
		"temperature":0.2,
		"presence_penalty":0.5,
		"frequency_penalty":0.1,
		"top_k":40
	}`)
	got, err := SanitizeChatBody("gemini", body)
	if err != nil {
		t.Fatal(err)
	}
	var obj map[string]any
	if err := json.Unmarshal(got, &obj); err != nil {
		t.Fatal(err)
	}
	if obj["temperature"] == nil {
		t.Fatal("temperature should pass")
	}
	if _, ok := obj["presence_penalty"]; ok {
		t.Fatal("presence_penalty must be stripped for gemini")
	}
	if _, ok := obj["frequency_penalty"]; ok {
		t.Fatal("frequency_penalty must be stripped for gemini")
	}
	if _, ok := obj["top_k"]; ok {
		t.Fatal("top_k must be stripped for gemini")
	}
}

func TestChatCompletionsOmitsModelUsesAuto(t *testing.T) {
	var sawModel string
	p := &mockProvider{
		name: "groq",
		models: []Model{{
			ID: "openai/gpt-oss-20b", Object: "model", OwnedBy: "openai", Provider: "groq", Free: true,
			Quota: &Quota{RemainingRPD: int64ptr(5)},
		}},
		chatFn: func(body json.RawMessage) (json.RawMessage, int, http.Header, error) {
			var req struct {
				Model       string  `json:"model"`
				Temperature float64 `json:"temperature"`
			}
			_ = json.Unmarshal(body, &req)
			sawModel = req.Model
			if req.Temperature != 0.5 {
				t.Fatalf("temperature=%v", req.Temperature)
			}
			return json.RawMessage(`{"id":"ok","choices":[]}`), 200, http.Header{}, nil
		},
	}
	reg := NewRegistry(nil, p)
	body := json.RawMessage(`{"messages":[{"role":"user","content":"hi"}],"temperature":0.5,"max_tokens":32}`)
	res, err := reg.ChatCompletions(context.Background(), body, RouteOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if sawModel != "openai/gpt-oss-20b" || res.Model != sawModel {
		t.Fatalf("auto model=%q res=%q", sawModel, res.Model)
	}
}
