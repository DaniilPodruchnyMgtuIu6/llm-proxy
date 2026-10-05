package provider

import (
	"encoding/json"
	"fmt"
)

// PresetFields are sampling defaults merged into a chat body when absent.
type PresetFields struct {
	Model            string
	Temperature      *float64
	TopP             *float64
	TopK             *int
	MaxTokens        *int
	PresencePenalty  *float64
	FrequencyPenalty *float64
}

// MergePresetIntoBody fills missing model/sampling fields from preset.
// Explicit non-null fields in body win. Empty string model is treated as missing.
func MergePresetIntoBody(body json.RawMessage, p PresetFields) (json.RawMessage, error) {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(body, &obj); err != nil {
		return nil, fmt.Errorf("invalid chat body: %w", err)
	}
	if obj == nil {
		obj = map[string]json.RawMessage{}
	}

	if !hasNonNull(obj, "model") || isEmptyStringJSON(obj["model"]) {
		model := p.Model
		if model == "" {
			model = "auto"
		}
		raw, _ := json.Marshal(model)
		obj["model"] = raw
	}
	setIfMissingFloat(obj, "temperature", p.Temperature)
	setIfMissingFloat(obj, "top_p", p.TopP)
	setIfMissingInt(obj, "top_k", p.TopK)
	setIfMissingInt(obj, "max_tokens", p.MaxTokens)
	setIfMissingFloat(obj, "presence_penalty", p.PresencePenalty)
	setIfMissingFloat(obj, "frequency_penalty", p.FrequencyPenalty)

	return json.Marshal(obj)
}

func hasNonNull(obj map[string]json.RawMessage, key string) bool {
	raw, ok := obj[key]
	if !ok {
		return false
	}
	return string(raw) != "null"
}

func isEmptyStringJSON(raw json.RawMessage) bool {
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return false
	}
	return s == ""
}

func setIfMissingFloat(obj map[string]json.RawMessage, key string, v *float64) {
	if v == nil || hasNonNull(obj, key) {
		return
	}
	raw, _ := json.Marshal(*v)
	obj[key] = raw
}

func setIfMissingInt(obj map[string]json.RawMessage, key string, v *int) {
	if v == nil || hasNonNull(obj, key) {
		return
	}
	raw, _ := json.Marshal(*v)
	obj[key] = raw
}
