package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

type UsageEvent struct {
	TS               time.Time
	Provider         string
	Model            string
	PoolID           string
	StatusCode       int
	LatencyMS        int64
	PromptTokens     int64
	CompletionTokens int64
	TotalTokens      int64
	ErrorType        string
	RequestID        string
}

type DayStat struct {
	Day            string `json:"day"`
	Provider       string `json:"provider"`
	Model          string `json:"model"`
	PoolID         string `json:"pool_id,omitempty"`
	SuccessCount   int64  `json:"success_count"`
	ErrorCount     int64  `json:"error_count"`
	RateLimitCount int64  `json:"rate_limit_count"`
	TokensIn       int64  `json:"tokens_in"`
	TokensOut      int64  `json:"tokens_out"`
}

type Summary struct {
	From           string    `json:"from"`
	To             string    `json:"to"`
	TotalSuccess   int64     `json:"total_success"`
	TotalErrors    int64     `json:"total_errors"`
	TotalRateLimit int64     `json:"total_rate_limit"`
	TokensIn       int64     `json:"tokens_in"`
	TokensOut      int64     `json:"tokens_out"`
	ByProvider     []DayStat `json:"by_provider"`
	ByModel        []DayStat `json:"by_model"`
}

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("mkdir data: %w", err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS usage_events (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  ts TEXT NOT NULL,
  provider TEXT NOT NULL,
  model TEXT NOT NULL,
  pool_id TEXT NOT NULL DEFAULT '',
  status_code INTEGER NOT NULL,
  latency_ms INTEGER NOT NULL DEFAULT 0,
  prompt_tokens INTEGER NOT NULL DEFAULT 0,
  completion_tokens INTEGER NOT NULL DEFAULT 0,
  total_tokens INTEGER NOT NULL DEFAULT 0,
  error_type TEXT NOT NULL DEFAULT '',
  request_id TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_usage_events_ts ON usage_events(ts);
CREATE INDEX IF NOT EXISTS idx_usage_events_provider ON usage_events(provider, ts);

CREATE TABLE IF NOT EXISTS daily_counters (
  day TEXT NOT NULL,
  provider TEXT NOT NULL,
  model TEXT NOT NULL,
  pool_id TEXT NOT NULL DEFAULT '',
  success_count INTEGER NOT NULL DEFAULT 0,
  error_count INTEGER NOT NULL DEFAULT 0,
  rate_limit_count INTEGER NOT NULL DEFAULT 0,
  tokens_in INTEGER NOT NULL DEFAULT 0,
  tokens_out INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (day, provider, model, pool_id)
);

CREATE TABLE IF NOT EXISTS provider_health (
  provider TEXT PRIMARY KEY,
  last_success_at TEXT,
  last_error_at TEXT,
  last_error TEXT,
  consecutive_errors INTEGER NOT NULL DEFAULT 0
);
`)
	return err
}

// DayBucket returns YYYY-MM-DD for the provider's reset timezone.
func DayBucket(provider string, now time.Time) string {
	switch strings.ToLower(provider) {
	case "gemini":
		loc, err := time.LoadLocation("America/Los_Angeles")
		if err != nil {
			return now.UTC().Format("2006-01-02")
		}
		return now.In(loc).Format("2006-01-02")
	default:
		return now.UTC().Format("2006-01-02")
	}
}

func (s *Store) RecordUsage(ctx context.Context, ev UsageEvent) error {
	if s == nil {
		return nil
	}
	if ev.TS.IsZero() {
		ev.TS = time.Now().UTC()
	}
	day := DayBucket(ev.Provider, ev.TS)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	_, err = tx.ExecContext(ctx, `
INSERT INTO usage_events(
  ts, provider, model, pool_id, status_code, latency_ms,
  prompt_tokens, completion_tokens, total_tokens, error_type, request_id
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		ev.TS.UTC().Format(time.RFC3339Nano),
		ev.Provider, ev.Model, nullPool(ev.PoolID), ev.StatusCode, ev.LatencyMS,
		ev.PromptTokens, ev.CompletionTokens, ev.TotalTokens, ev.ErrorType, ev.RequestID,
	)
	if err != nil {
		return err
	}

	success, errors, rateLimits := int64(0), int64(0), int64(0)
	if ev.StatusCode >= 200 && ev.StatusCode < 300 {
		success = 1
	} else {
		errors = 1
		if ev.StatusCode == 429 || ev.ErrorType == "rate_limit" {
			rateLimits = 1
		}
	}

	_, err = tx.ExecContext(ctx, `
INSERT INTO daily_counters(day, provider, model, pool_id, success_count, error_count, rate_limit_count, tokens_in, tokens_out)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(day, provider, model, pool_id) DO UPDATE SET
  success_count = success_count + excluded.success_count,
  error_count = error_count + excluded.error_count,
  rate_limit_count = rate_limit_count + excluded.rate_limit_count,
  tokens_in = tokens_in + excluded.tokens_in,
  tokens_out = tokens_out + excluded.tokens_out
`, day, ev.Provider, ev.Model, nullPool(ev.PoolID), success, errors, rateLimits, ev.PromptTokens, ev.CompletionTokens)
	if err != nil {
		return err
	}

	if success == 1 {
		_, err = tx.ExecContext(ctx, `
INSERT INTO provider_health(provider, last_success_at, consecutive_errors)
VALUES (?, ?, 0)
ON CONFLICT(provider) DO UPDATE SET
  last_success_at = excluded.last_success_at,
  consecutive_errors = 0
`, ev.Provider, ev.TS.UTC().Format(time.RFC3339Nano))
	} else {
		_, err = tx.ExecContext(ctx, `
INSERT INTO provider_health(provider, last_error_at, last_error, consecutive_errors)
VALUES (?, ?, ?, 1)
ON CONFLICT(provider) DO UPDATE SET
  last_error_at = excluded.last_error_at,
  last_error = excluded.last_error,
  consecutive_errors = consecutive_errors + 1
`, ev.Provider, ev.TS.UTC().Format(time.RFC3339Nano), truncate(ev.ErrorType, 200))
	}
	if err != nil {
		return err
	}

	return tx.Commit()
}

func (s *Store) SuccessCount(ctx context.Context, day, provider, model, poolID string) (int64, error) {
	if s == nil {
		return 0, nil
	}
	var n int64
	err := s.db.QueryRowContext(ctx, `
SELECT COALESCE(SUM(success_count), 0) FROM daily_counters
WHERE day = ? AND provider = ? AND model = ? AND pool_id = ?
`, day, provider, model, nullPool(poolID)).Scan(&n)
	return n, err
}

func (s *Store) PoolSuccessCount(ctx context.Context, day, poolID string) (int64, error) {
	if s == nil || poolID == "" {
		return 0, nil
	}
	var n int64
	err := s.db.QueryRowContext(ctx, `
SELECT COALESCE(SUM(success_count), 0) FROM daily_counters
WHERE day = ? AND pool_id = ?
`, day, poolID).Scan(&n)
	return n, err
}

func (s *Store) Summary(ctx context.Context, fromDay, toDay, provider string) (Summary, error) {
	out := Summary{From: fromDay, To: toDay}
	if s == nil {
		return out, nil
	}

	where := `day >= ? AND day <= ?`
	args := []any{fromDay, toDay}
	if provider != "" {
		where += ` AND provider = ?`
		args = append(args, provider)
	}

	err := s.db.QueryRowContext(ctx, `
SELECT COALESCE(SUM(success_count),0), COALESCE(SUM(error_count),0),
       COALESCE(SUM(rate_limit_count),0), COALESCE(SUM(tokens_in),0), COALESCE(SUM(tokens_out),0)
FROM daily_counters WHERE `+where, args...).Scan(
		&out.TotalSuccess, &out.TotalErrors, &out.TotalRateLimit, &out.TokensIn, &out.TokensOut,
	)
	if err != nil {
		return out, err
	}

	rows, err := s.db.QueryContext(ctx, `
SELECT provider, '' AS model, '' AS pool_id,
       SUM(success_count), SUM(error_count), SUM(rate_limit_count), SUM(tokens_in), SUM(tokens_out)
FROM daily_counters WHERE `+where+`
GROUP BY provider ORDER BY provider`, args...)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var st DayStat
		if err := rows.Scan(&st.Provider, &st.Model, &st.PoolID, &st.SuccessCount, &st.ErrorCount, &st.RateLimitCount, &st.TokensIn, &st.TokensOut); err != nil {
			return out, err
		}
		out.ByProvider = append(out.ByProvider, st)
	}

	rows2, err := s.db.QueryContext(ctx, `
SELECT provider, model, pool_id,
       SUM(success_count), SUM(error_count), SUM(rate_limit_count), SUM(tokens_in), SUM(tokens_out)
FROM daily_counters WHERE `+where+`
GROUP BY provider, model, pool_id
ORDER BY SUM(success_count) DESC
LIMIT 100`, args...)
	if err != nil {
		return out, err
	}
	defer rows2.Close()
	for rows2.Next() {
		var st DayStat
		if err := rows2.Scan(&st.Provider, &st.Model, &st.PoolID, &st.SuccessCount, &st.ErrorCount, &st.RateLimitCount, &st.TokensIn, &st.TokensOut); err != nil {
			return out, err
		}
		out.ByModel = append(out.ByModel, st)
	}
	return out, nil
}

func nullPool(v string) string {
	if v == "" {
		return ""
	}
	return v
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
