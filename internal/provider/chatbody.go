package provider

import (
	"encoding/json"
	"fmt"
)

// Common OpenAI-compatible chat fields accepted by the proxy API.
// Provider-specific allowlists decide what is forwarded upstream.
var chatFieldMeta = map[string]struct {
	all      bool
	gemini   bool
	groq     bool
	openrouter bool
}{
	"messages":              {all: true},
	"model":                 {all: true},
	"temperature":           {all: true},
	"top_p":                 {all: true},
	"max_tokens":            {all: true},
	"max_completion_tokens": {all: true},
	"n":                     {all: true},
	"stop":                  {all: true},
	"stream":                {all: true},
	// Gemini OpenAI-compat rejects these ("Unknown name ... Cannot find field").
	"presence_penalty":  {gemini: false, groq: true, openrouter: true},
	"frequency_penalty": {gemini: false, groq: true, openrouter: true},

	"seed":                {all: true},
	"user":                {all: true},
	"response_format":     {all: true},
	"tools":               {all: true},
	"tool_choice":         {all: true},
	"parallel_tool_calls": {all: true},

	// Not in core OpenAI chat schema; useful where upstream accepts it.
	"top_k": {gemini: false, groq: false, openrouter: true},

	// Gemini OpenAI-compat extras.
	"reasoning_effort": {gemini: true, groq: false, openrouter: true},
	"modalities":       {gemini: true, groq: false, openrouter: false},

	// Often rejected by Groq; OpenRouter may pass through.
	"logit_bias":   {gemini: false, groq: false, openrouter: true},
	"logprobs":     {gemini: false, groq: false, openrouter: true},
	"top_logprobs": {gemini: false, groq: false, openrouter: true},
}

// SanitizeChatBody keeps only fields the target provider is known to accept.
// Unknown keys are dropped. Proxy-only route fields should already be stripped by the API layer.
func SanitizeChatBody(providerName string, body json.RawMessage) (json.RawMessage, error) {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(body, &obj); err != nil {
		return nil, fmt.Errorf("invalid chat body: %w", err)
	}

	out := make(map[string]json.RawMessage, len(obj))
	for k, v := range obj {
		if !chatFieldAllowed(providerName, k) {
			continue
		}
		out[k] = v
	}

	if msgs, ok := out["messages"]; ok {
		cleaned, err := sanitizeMessages(providerName, msgs)
		if err != nil {
			return nil, err
		}
		out["messages"] = cleaned
	}

	// Groq only accepts n=1; omit other values instead of failing upstream.
	if providerName == "groq" {
		if raw, ok := out["n"]; ok {
			var n int
			if err := json.Unmarshal(raw, &n); err == nil && n != 1 {
				delete(out, "n")
			}
		}
	}

	return json.Marshal(out)
}

func chatFieldAllowed(providerName, field string) bool {
	meta, ok := chatFieldMeta[field]
	if !ok {
		return false
	}
	if meta.all {
		return true
	}
	switch providerName {
	case "gemini":
		return meta.gemini
	case "groq":
		return meta.groq
	case "openrouter":
		return meta.openrouter
	default:
		// Unknown provider: forward common OpenAI fields only.
		return meta.all
	}
}

func sanitizeMessages(providerName string, raw json.RawMessage) (json.RawMessage, error) {
	var msgs []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &msgs); err != nil {
		return raw, nil // leave as-is if not a JSON array of objects
	}
	if providerName != "groq" {
		return raw, nil
	}
	for i := range msgs {
		delete(msgs[i], "name") // Groq rejects messages[].name
	}
	return json.Marshal(msgs)
}
