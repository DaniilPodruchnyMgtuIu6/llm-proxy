//go:build integration

package store

import (
	"context"
	"net/http"
	"os"
	"testing"
	"time"
)

func TestIntegrationMigrationsAndRateLimits(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = os.Getenv("DB_DSN")
	}
	if dsn == "" {
		// Default local docker-compose DSN.
		dsn = "postgres://llmproxy:llmproxy@localhost:5432/llmproxy?sslmode=disable"
	}

	st, err := Open(dsn)
	if err != nil {
		t.Skipf("postgres unavailable: %v", err)
	}
	defer st.Close()

	ctx := context.Background()
	h := http.Header{}
	h.Set("x-ratelimit-limit-requests", "30")
	h.Set("x-ratelimit-remaining-requests", "28")
	h.Set("x-ratelimit-limit-tokens", "6000")
	h.Set("x-ratelimit-remaining-tokens", "5500")

	rl := ParseRateLimitHeaders(h)
	rl.Provider = "groq"
	rl.Model = "openai/gpt-oss-20b"
	rl.UpdatedAt = time.Now().UTC()
	if err := st.UpsertRateLimit(ctx, rl); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	got, ok, err := st.GetRateLimit(ctx, "groq", "openai/gpt-oss-20b")
	if err != nil || !ok {
		t.Fatalf("get: ok=%v err=%v", ok, err)
	}
	if got.RemainingRequests == nil || *got.RemainingRequests != 28 {
		t.Fatalf("remaining=%v", got.RemainingRequests)
	}
	if got.RemainingTokens == nil || *got.RemainingTokens != 5500 {
		t.Fatalf("tokens=%v", got.RemainingTokens)
	}

	if err := st.RecordUsage(ctx, UsageEvent{
		TS: time.Now().UTC(), Provider: "groq", Model: "openai/gpt-oss-20b",
		StatusCode: 200, LatencyMS: 12, PromptTokens: 1, CompletionTokens: 2, TotalTokens: 3,
	}); err != nil {
		t.Fatalf("record: %v", err)
	}
}
