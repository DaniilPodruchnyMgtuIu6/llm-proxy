package provider

import "testing"

func TestIsChatCapableFiltersSpecialty(t *testing.T) {
	cases := []struct {
		provider string
		id       string
		want     bool
	}{
		{"gemini", "gemini-3.5-flash-lite", true},
		{"gemini", "gemini-pro-latest", true},
		{"gemini", "gemma-4-31b-it", true},
		{"gemini", "gemini-embedding-001", false},
		{"gemini", "gemini-2.5-flash-image", false},
		{"gemini", "gemini-2.5-flash-preview-tts", false},
		{"gemini", "veo-3.1-generate-preview", false},
		{"gemini", "lyria-3.5", false},
		{"gemini", "gemini-3.1-pro-preview-customtools", false},
		{"groq", "openai/gpt-oss-20b", true},
		{"groq", "whisper-large-v3", false},
		{"groq", "meta-llama/llama-prompt-guard-2-22m", false},
		{"groq", "canopylabs/orpheus-v1-english", false},
		{"openrouter", "nvidia/nemotron-3-ultra-550b-a55b:free", true},
		{"openrouter", "openrouter/free", true},
	}
	for _, tc := range cases {
		got := IsChatCapable(tc.provider, tc.id)
		if got != tc.want {
			t.Fatalf("%s/%s: got %v want %v", tc.provider, tc.id, got, tc.want)
		}
	}
}
