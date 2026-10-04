package store

import (
	"net/http"
	"testing"
)

func TestParseRateLimitHeaders(t *testing.T) {
	h := http.Header{}
	h.Set("x-ratelimit-limit-requests", "30")
	h.Set("x-ratelimit-remaining-requests", "29")
	h.Set("x-ratelimit-reset-requests", "2m0s")
	h.Set("X-Ratelimit-Limit-Tokens", "6000")
	h.Set("X-Ratelimit-Remaining-Tokens", "5900.0")
	h.Set("X-Ratelimit-Reset-Tokens", "1m30s")

	rl := ParseRateLimitHeaders(h)
	if rl.LimitRequests == nil || *rl.LimitRequests != 30 {
		t.Fatalf("limit requests: %#v", rl.LimitRequests)
	}
	if rl.RemainingRequests == nil || *rl.RemainingRequests != 29 {
		t.Fatalf("remaining requests: %#v", rl.RemainingRequests)
	}
	if rl.ResetRequests != "2m0s" {
		t.Fatalf("reset requests: %q", rl.ResetRequests)
	}
	if rl.LimitTokens == nil || *rl.LimitTokens != 6000 {
		t.Fatalf("limit tokens: %#v", rl.LimitTokens)
	}
	if rl.RemainingTokens == nil || *rl.RemainingTokens != 5900 {
		t.Fatalf("remaining tokens: %#v", rl.RemainingTokens)
	}
	if rl.ResetTokens != "1m30s" {
		t.Fatalf("reset tokens: %q", rl.ResetTokens)
	}
}

func TestParseRateLimitHeadersEmpty(t *testing.T) {
	rl := ParseRateLimitHeaders(nil)
	if rl.LimitRequests != nil || rl.RemainingRequests != nil {
		t.Fatalf("expected empty: %#v", rl)
	}
}
