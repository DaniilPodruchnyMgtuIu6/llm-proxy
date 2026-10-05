package provider

import (
	"encoding/json"
	"testing"
)

func TestMergePresetIntoBodyFillsMissing(t *testing.T) {
	body := json.RawMessage(`{"messages":[{"role":"user","content":"hi"}]}`)
	temp := 0.2
	maxTok := 128
	got, err := MergePresetIntoBody(body, PresetFields{
		Model:       "gemini-3.5-flash-lite",
		Temperature: &temp,
		MaxTokens:   &maxTok,
	})
	if err != nil {
		t.Fatal(err)
	}
	var obj map[string]any
	if err := json.Unmarshal(got, &obj); err != nil {
		t.Fatal(err)
	}
	if obj["model"] != "gemini-3.5-flash-lite" {
		t.Fatalf("model=%v", obj["model"])
	}
	if obj["temperature"] != 0.2 {
		t.Fatalf("temperature=%v", obj["temperature"])
	}
	if int(obj["max_tokens"].(float64)) != 128 {
		t.Fatalf("max_tokens=%v", obj["max_tokens"])
	}
}

func TestMergePresetIntoBodyKeepsExplicit(t *testing.T) {
	body := json.RawMessage(`{"messages":[{"role":"user","content":"hi"}],"model":"groq-x","temperature":0.9}`)
	temp := 0.1
	got, err := MergePresetIntoBody(body, PresetFields{
		Model:       "gemini-y",
		Temperature: &temp,
	})
	if err != nil {
		t.Fatal(err)
	}
	var obj map[string]any
	_ = json.Unmarshal(got, &obj)
	if obj["model"] != "groq-x" {
		t.Fatalf("model overridden: %v", obj["model"])
	}
	if obj["temperature"] != 0.9 {
		t.Fatalf("temperature overridden: %v", obj["temperature"])
	}
}
