package provider

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
)

// ValidateChatBody checks sampling/limit fields before routing upstream.
// Unknown keys are ignored here (SanitizeChatBody drops them later).
func ValidateChatBody(body json.RawMessage) error {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(body, &obj); err != nil {
		return fmt.Errorf("invalid JSON body")
	}
	return validateSamplingFields(obj)
}

type samplingValues struct {
	Temperature         *float64
	TopP                *float64
	TopK                *int
	MaxTokens           *int
	MaxCompletionTokens *int
	PresencePenalty     *float64
	FrequencyPenalty    *float64
	N                   *int
}

func validateSamplingFields(obj map[string]json.RawMessage) error {
	v := samplingValues{}
	if err := decodeOptFloat(obj, "temperature", &v.Temperature); err != nil {
		return err
	}
	if err := decodeOptFloat(obj, "top_p", &v.TopP); err != nil {
		return err
	}
	if err := decodeOptInt(obj, "top_k", &v.TopK); err != nil {
		return err
	}
	if err := decodeOptInt(obj, "max_tokens", &v.MaxTokens); err != nil {
		return err
	}
	if err := decodeOptInt(obj, "max_completion_tokens", &v.MaxCompletionTokens); err != nil {
		return err
	}
	if err := decodeOptFloat(obj, "presence_penalty", &v.PresencePenalty); err != nil {
		return err
	}
	if err := decodeOptFloat(obj, "frequency_penalty", &v.FrequencyPenalty); err != nil {
		return err
	}
	if err := decodeOptInt(obj, "n", &v.N); err != nil {
		return err
	}
	return validateSampling(v)
}

func validateSampling(v samplingValues) error {
	if v.Temperature != nil {
		if math.IsNaN(*v.Temperature) || math.IsInf(*v.Temperature, 0) || *v.Temperature < 0 || *v.Temperature > 2 {
			return fmt.Errorf("temperature must be in [0, 2], got %v", *v.Temperature)
		}
	}
	if v.TopP != nil {
		if math.IsNaN(*v.TopP) || math.IsInf(*v.TopP, 0) || *v.TopP < 0 || *v.TopP > 1 {
			return fmt.Errorf("top_p must be in [0.0, 1.0], got %v", *v.TopP)
		}
	}
	if v.TopK != nil {
		if *v.TopK < 1 || *v.TopK > 200 {
			return fmt.Errorf("top_k must be an integer in [1, 200], got %d", *v.TopK)
		}
	}
	if v.MaxTokens != nil {
		if *v.MaxTokens < 1 || *v.MaxTokens > 128_000 {
			return fmt.Errorf("max_tokens must be in [1, 128000], got %d", *v.MaxTokens)
		}
		if *v.MaxTokens < 8 {
			return fmt.Errorf("max_tokens=%d is too small for a useful chat reply; use at least 8 (recommended >= 64)", *v.MaxTokens)
		}
	}
	if v.MaxCompletionTokens != nil {
		if *v.MaxCompletionTokens < 1 || *v.MaxCompletionTokens > 128_000 {
			return fmt.Errorf("max_completion_tokens must be in [1, 128000], got %d", *v.MaxCompletionTokens)
		}
		if *v.MaxCompletionTokens < 8 {
			return fmt.Errorf("max_completion_tokens=%d is too small for a useful chat reply; use at least 8", *v.MaxCompletionTokens)
		}
	}
	if v.PresencePenalty != nil {
		if math.IsNaN(*v.PresencePenalty) || math.IsInf(*v.PresencePenalty, 0) || *v.PresencePenalty < -2 || *v.PresencePenalty > 2 {
			return fmt.Errorf("presence_penalty must be in [-2, 2], got %v", *v.PresencePenalty)
		}
	}
	if v.FrequencyPenalty != nil {
		if math.IsNaN(*v.FrequencyPenalty) || math.IsInf(*v.FrequencyPenalty, 0) || *v.FrequencyPenalty < -2 || *v.FrequencyPenalty > 2 {
			return fmt.Errorf("frequency_penalty must be in [-2, 2], got %v", *v.FrequencyPenalty)
		}
	}
	if v.N != nil && *v.N != 1 {
		return fmt.Errorf("n must be 1 (got %d)", *v.N)
	}
	return nil
}

// ValidateDefaults validates gateway UI defaults (same ranges as chat).
func ValidateDefaults(model string, temperature, topP, presence, frequency *float64, topK, maxTokens *int) error {
	_ = model
	v := samplingValues{
		Temperature:      temperature,
		TopP:             topP,
		TopK:             topK,
		MaxTokens:        maxTokens,
		PresencePenalty:  presence,
		FrequencyPenalty: frequency,
	}
	return validateSampling(v)
}

func decodeOptFloat(obj map[string]json.RawMessage, key string, dst **float64) error {
	raw, ok := obj[key]
	if !ok || string(raw) == "null" {
		return nil
	}
	var n float64
	if err := json.Unmarshal(raw, &n); err != nil {
		return fmt.Errorf("%s must be a number", key)
	}
	*dst = &n
	return nil
}

func decodeOptInt(obj map[string]json.RawMessage, key string, dst **int) error {
	raw, ok := obj[key]
	if !ok || string(raw) == "null" {
		return nil
	}
	var f float64
	if err := json.Unmarshal(raw, &f); err != nil {
		return fmt.Errorf("%s must be a number", key)
	}
	if f != math.Trunc(f) {
		return fmt.Errorf("%s must be an integer, got %v", key, f)
	}
	n := int(f)
	*dst = &n
	return nil
}

// BumpMaxTokens raises token limits to at least min (used after empty length replies).
func BumpMaxTokens(body json.RawMessage, min int) (json.RawMessage, error) {
	if min < 1 {
		min = 1
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(body, &obj); err != nil {
		return body, err
	}
	raised := false
	for _, key := range []string{"max_tokens", "max_completion_tokens"} {
		raw, ok := obj[key]
		if !ok || string(raw) == "null" {
			continue
		}
		var n int
		if err := json.Unmarshal(raw, &n); err != nil {
			continue
		}
		if n < min {
			b, _ := json.Marshal(min)
			obj[key] = b
			raised = true
		}
	}
	if !raised {
		if _, hasMax := obj["max_tokens"]; !hasMax {
			if _, hasComp := obj["max_completion_tokens"]; !hasComp {
				b, _ := json.Marshal(min)
				obj["max_tokens"] = b
			}
		}
	}
	return json.Marshal(obj)
}

// IsEmptyLengthCompletion detects "success" replies that are useless
// (no assistant text, stopped only because max_tokens was exhausted).
func IsEmptyLengthCompletion(body json.RawMessage) bool {
	var resp struct {
		Choices []struct {
			FinishReason string `json:"finish_reason"`
			Message      struct {
				Content any `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &resp); err != nil || len(resp.Choices) == 0 {
		return false
	}
	c := resp.Choices[0]
	if !strings.EqualFold(c.FinishReason, "length") {
		return false
	}
	return strings.TrimSpace(contentString(c.Message.Content)) == ""
}

func contentString(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	default:
		b, _ := json.Marshal(t)
		s := strings.TrimSpace(string(b))
		if s == "null" || s == `""` {
			return ""
		}
		return s
	}
}
