package ranking

import (
	"regexp"
	"strconv"
	"strings"
)

var (
	reGeminiVer = regexp.MustCompile(`gemini-(\d+)(?:\.(\d+))?`)
	reParamsB   = regexp.MustCompile(`(?i)(\d+(?:\.\d+)?)b`)
)

// Score returns a quality score (higher = smarter / preferred first).
// Cross-provider scale ~0..10000 for chat; specialty models lower.
func Score(provider, modelID string) int {
	id := strings.ToLower(strings.TrimSpace(modelID))
	switch strings.ToLower(provider) {
	case "gemini":
		return scoreGemini(id)
	case "groq":
		return scoreGroq(id)
	case "openrouter":
		return scoreOpenRouter(id)
	default:
		return 100
	}
}

func scoreGemini(id string) int {
	ver := geminiVersion(id) // e.g. 3.8 -> 380

	switch {
	case strings.Contains(id, "embedding"),
		strings.Contains(id, "tts"),
		strings.Contains(id, "lyria"),
		strings.Contains(id, "veo"),
		strings.Contains(id, "imagen"),
		strings.Contains(id, "transcribe"),
		strings.Contains(id, "robotics"),
		strings.Contains(id, "computer-use"),
		strings.Contains(id, "aqa"):
		return 800 + ver/10
	case strings.Contains(id, "image"), strings.Contains(id, "banana"):
		return 1500 + ver/10
	case strings.Contains(id, "live"):
		return 2000 + ver/10
	case strings.Contains(id, "gemma"):
		base := 5600
		if strings.Contains(id, "31b") {
			base = 6200
		}
		return base
	}

	// Text chat family: Pro > Flash > Flash-Lite
	tier := 5000
	switch {
	case strings.Contains(id, "pro"):
		tier = 9200
	case strings.Contains(id, "flash-lite"), strings.Contains(id, "flash_lite"):
		tier = 6400
	case strings.Contains(id, "flash"):
		tier = 7800
	}

	// Prefer explicit versions over *-latest aliases slightly under same tier.
	if strings.Contains(id, "latest") {
		tier -= 80
	}
	if strings.Contains(id, "preview") {
		tier -= 40
	}

	return tier + ver
}

func geminiVersion(id string) int {
	m := reGeminiVer.FindStringSubmatch(id)
	if m == nil {
		if strings.Contains(id, "flash-latest") || strings.Contains(id, "pro-latest") {
			return 350 // treat latest as mid-3.x
		}
		return 100
	}
	major, _ := strconv.Atoi(m[1])
	minor := 0
	if m[2] != "" {
		minor, _ = strconv.Atoi(m[2])
	}
	return major*100 + minor
}

func scoreGroq(id string) int {
	switch {
	case id == "openai/gpt-oss-120b":
		return 8600
	case id == "openai/gpt-oss-20b":
		return 7400
	case id == "qwen/qwen3.8-27b":
		return 7200
	case id == "openai/gpt-oss-safeguard-20b":
		return 4200
	case id == "allam-2-7b":
		return 4800
	case strings.Contains(id, "whisper"):
		return 1800
	case strings.Contains(id, "orpheus"):
		return 1600
	case strings.Contains(id, "prompt-guard"):
		return 900
	default:
		return 3000 + paramsBonus(id)
	}
}

func scoreOpenRouter(id string) int {
	if id == "openrouter/free" {
		// Convenient auto-router: mid priority (not first, not last).
		return 5800
	}

	base := 5000
	switch {
	case strings.Contains(id, "ultra") || strings.Contains(id, "550b"):
		base = 9000
	case strings.Contains(id, "120b") || strings.Contains(id, "super"):
		base = 8400
	case strings.Contains(id, "pro"):
		base = 8000
	case strings.Contains(id, "31b") || strings.Contains(id, "32b") || strings.Contains(id, "27b"):
		base = 7600
	case strings.Contains(id, "nemotron") && strings.Contains(id, "reasoning"):
		base = 8200
	case strings.Contains(id, "gemma-4-31b"):
		base = 7500
	case strings.Contains(id, "gemma-4-26b"):
		base = 7000
	case strings.Contains(id, "qwen"):
		base = 7300
	case strings.Contains(id, "mini") || strings.Contains(id, "lite") || strings.Contains(id, "nano") || strings.Contains(id, "small"):
		base = 5200
	case strings.Contains(id, "safety") || strings.Contains(id, "guard"):
		base = 2500
	case strings.Contains(id, "lyria") || strings.Contains(id, "whisper") || strings.Contains(id, "tts"):
		base = 1400
	}

	base += paramsBonus(id)
	if strings.HasSuffix(id, ":free") {
		base += 20
	}
	if strings.Contains(id, "preview") || strings.Contains(id, "alpha") {
		base -= 30
	}
	return base
}

func paramsBonus(id string) int {
	m := reParamsB.FindStringSubmatch(id)
	if m == nil {
		return 0
	}
	f, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return 0
	}
	// Cap contribution so 550b doesn't explode past intentional tiers.
	bonus := int(f * 2)
	if bonus > 400 {
		bonus = 400
	}
	return bonus
}
