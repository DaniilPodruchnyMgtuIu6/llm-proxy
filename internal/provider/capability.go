package provider

import "strings"

// IsChatCapable reports whether a model id is suitable for /v1/chat/completions.
// Upstream /models lists often include embeddings, image, TTS, ASR, video, etc.
func IsChatCapable(providerName, modelID string) bool {
	id := strings.ToLower(strings.TrimSpace(modelID))
	if id == "" {
		return false
	}
	if isNonChatModelID(id) {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(providerName)) {
	case "gemini":
		return isGeminiChatID(id)
	case "groq":
		return isGroqChatID(id)
	case "openrouter":
		return isOpenRouterChatID(id)
	default:
		return true
	}
}

func isNonChatModelID(id string) bool {
	needles := []string{
		"embedding", "embed-",
		"tts", "whisper", "transcribe", "asr",
		"lyria", "music",
		"veo", "imagen", "image", "banana",
		"robotics", "computer-use", "aqa",
		"prompt-guard", "moderation",
		"realtime", "native-audio",
	}
	for _, n := range needles {
		if strings.Contains(id, n) {
			return true
		}
	}
	return false
}

func isGeminiChatID(id string) bool {
	// Keep text chat / gemma instruct; drop live/preview research agents.
	if strings.Contains(id, "live") && !strings.Contains(id, "flash") {
		return false
	}
	if strings.HasPrefix(id, "antigravity-") || strings.HasPrefix(id, "deep-research-") {
		return false
	}
	if strings.Contains(id, "customtools") {
		// Often chat-incompatible or restricted; keep out of default catalog.
		return false
	}
	// Accept gemini-* and gemma-* text families.
	if strings.HasPrefix(id, "gemini-") || strings.HasPrefix(id, "gemma-") {
		return true
	}
	return false
}

func isGroqChatID(id string) bool {
	// Audio / TTS / classifiers already caught by isNonChatModelID.
	// Keep known chat OSS and other text models from Groq catalog.
	switch {
	case strings.Contains(id, "whisper"),
		strings.Contains(id, "orpheus"),
		strings.Contains(id, "prompt-guard"):
		return false
	default:
		return true
	}
}

func isOpenRouterChatID(id string) bool {
	if id == "openrouter/free" {
		return true
	}
	// Free router aliases and :free text models only.
	if !strings.HasSuffix(id, ":free") && id != "openrouter/free" {
		// ListModels already filters free; keep defensive.
		return strings.Contains(id, "/")
	}
	return true
}
