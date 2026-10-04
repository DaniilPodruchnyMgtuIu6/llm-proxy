package quota

import (
	"strings"
	"time"

	"github.com/llm-proxy/llm-proxy/internal/provider"
)

const (
	PoolOpenRouterFree = "openrouter:free"
)

// GeminiQuota returns a static Free-tier estimate for a Gemini model id.
// Exact RPM/RPD must be verified in AI Studio; remaining is not available via API.
func GeminiQuota(modelID string) *provider.Quota {
	id := strings.ToLower(modelID)
	q := &provider.Quota{
		Scope:        "per_model",
		Source:       "static_catalog",
		Confidence:   "estimate",
		Tier:         "free",
		ResetRPDHint: "midnight_pacific",
		ResetsAt:     NextMidnightPacific(time.Now()),
		Notes:        "Limits vary by model/account; verify in AI Studio. Remaining not exposed by API.",
	}

	switch {
	case strings.Contains(id, "flash-lite"):
		q.RPM, q.RPD, q.TPM = Int64(15), Int64(1000), Int64(250_000)
	case strings.Contains(id, "flash"):
		q.RPM, q.RPD, q.TPM = Int64(10), Int64(250), Int64(250_000)
	case strings.Contains(id, "pro"):
		q.RPM, q.RPD, q.TPM = Int64(5), Int64(100), Int64(250_000)
	default:
		q.RPM, q.RPD, q.TPM = Int64(10), Int64(250), Int64(250_000)
		q.Confidence = "unknown"
		q.Notes = "No specific free-tier row in catalog; estimate only. Check AI Studio."
	}
	return q
}

// GroqQuota returns Free Plan limits from Groq docs (org-level, per model).
func GroqQuota(modelID string) *provider.Quota {
	q := &provider.Quota{
		Scope:        "per_model",
		Source:       "static_catalog",
		Confidence:   "exact",
		Tier:         "free",
		ResetRPDHint: "sliding_window",
		Notes:        "Org-level free plan limits from Groq docs; remaining via response headers after chat calls.",
	}

	switch modelID {
	case "openai/gpt-oss-20b", "openai/gpt-oss-120b", "qwen/qwen3.8-27b":
		q.RPM, q.RPD, q.TPM, q.TPD = Int64(30), Int64(1000), Int64(8000), Int64(200_000)
	case "openai/gpt-oss-safeguard-20b":
		q.RPM, q.RPD, q.TPM, q.TPD = Int64(5), Int64(1000), Int64(2000), Int64(200_000)
	case "whisper-large-v3", "whisper-large-v3-turbo":
		q.RPM, q.RPD = Int64(20), Int64(2000)
		q.Notes = "Audio model; ASH/ASD limits also apply (see Groq docs)."
	case "canopylabs/orpheus-v1-english", "canopylabs/orpheus-arabic-saudi":
		q.RPM, q.RPD, q.TPM, q.TPD = Int64(10), Int64(100), Int64(1200), Int64(3600)
	case "meta-llama/llama-prompt-guard-2-22m", "meta-llama/llama-prompt-guard-2-86m":
		q.RPM, q.RPD, q.TPM, q.TPD = Int64(30), Int64(14400), Int64(15000), Int64(500_000)
		q.Notes = "Classifier, not a general chat model."
	default:
		q.Confidence = "unknown"
		q.RPM, q.RPD = nil, nil
		q.Notes = "Model not in free-plan catalog table; check Console → Limits."
	}
	return q
}

// OpenRouterFreePoolBase is the shared free-model request pool (before live fill).
func OpenRouterFreePoolBase() *provider.Quota {
	return &provider.Quota{
		RPM:          Int64(20),
		RPD:          Int64(50), // default low tier; live /key may raise to 1000
		Scope:        "shared_pool",
		PoolID:       PoolOpenRouterFree,
		Source:       "static_catalog",
		Confidence:   "estimate",
		Tier:         "free",
		ResetRPDHint: "midnight_utc",
		ResetsAt:     NextMidnightUTC(time.Now()),
		Notes:        "Shared across all :free models. 50 RPD if <~$10 credits purchased all-time, else 1000 RPD. Live remaining from GET /api/v1/key.",
	}
}

func IsGeminiRecommended(modelID string) bool {
	switch modelID {
	case "gemini-3.5-flash-lite", "gemini-flash-lite-latest", "gemini-3.5-flash", "gemini-flash-latest":
		return true
	default:
		return false
	}
}

func IsGroqRecommended(modelID string) bool {
	switch modelID {
	case "openai/gpt-oss-20b", "openai/gpt-oss-120b", "qwen/qwen3.8-27b":
		return true
	default:
		return false
	}
}

func IsOpenRouterRecommended(modelID string) bool {
	return modelID == "openrouter/free" || strings.HasSuffix(modelID, ":free")
}
