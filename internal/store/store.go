package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

type Store struct {
	db *sql.DB
}

type UsageEvent struct {
	TS               time.Time
	Provider         string
	Model            string
	PoolID           string
	Preset           string
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

// Open connects to PostgreSQL using a libpq/pgx DSN.
// Example: postgres://llmproxy:llmproxy@localhost:5432/llmproxy?sslmode=disable
func Open(databaseURL string) (*Store, error) {
	if strings.TrimSpace(databaseURL) == "" {
		return nil, fmt.Errorf("postgres DSN is empty (check DB_HOST/DB_PORT/DB_USER/DB_NAME)")
	}
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(time.Hour)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	s := &Store{db: db}
	if err := s.migrate(ctx); err != nil {
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
  prompt_tokens, completion_tokens, total_tokens, error_type, request_id, preset
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
		ev.TS.UTC(),
		ev.Provider, ev.Model, nullPool(ev.PoolID), ev.StatusCode, ev.LatencyMS,
		ev.PromptTokens, ev.CompletionTokens, ev.TotalTokens, ev.ErrorType, ev.RequestID,
		nullPool(ev.Preset),
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
VALUES ($1::date, $2, $3, $4, $5, $6, $7, $8, $9)
ON CONFLICT(day, provider, model, pool_id) DO UPDATE SET
  success_count = daily_counters.success_count + EXCLUDED.success_count,
  error_count = daily_counters.error_count + EXCLUDED.error_count,
  rate_limit_count = daily_counters.rate_limit_count + EXCLUDED.rate_limit_count,
  tokens_in = daily_counters.tokens_in + EXCLUDED.tokens_in,
  tokens_out = daily_counters.tokens_out + EXCLUDED.tokens_out
`, day, ev.Provider, ev.Model, nullPool(ev.PoolID), success, errors, rateLimits, ev.PromptTokens, ev.CompletionTokens)
	if err != nil {
		return err
	}

	if success == 1 {
		_, err = tx.ExecContext(ctx, `
INSERT INTO provider_health(provider, last_success_at, consecutive_errors)
VALUES ($1, $2, 0)
ON CONFLICT(provider) DO UPDATE SET
  last_success_at = EXCLUDED.last_success_at,
  consecutive_errors = 0
`, ev.Provider, ev.TS.UTC())
	} else if affectsCircuit(ev) {
		// Only hard upstream failures trip the circuit. 429 / 400 / 404 are
		// model- or quota-local and must not disable the whole provider.
		_, err = tx.ExecContext(ctx, `
INSERT INTO provider_health(provider, last_error_at, last_error, consecutive_errors)
VALUES ($1, $2, $3, 1)
ON CONFLICT(provider) DO UPDATE SET
  last_error_at = EXCLUDED.last_error_at,
  last_error = EXCLUDED.last_error,
  consecutive_errors = provider_health.consecutive_errors + 1
`, ev.Provider, ev.TS.UTC(), truncate(ev.ErrorType, 200))
	} else {
		_, err = tx.ExecContext(ctx, `
INSERT INTO provider_health(provider, last_error_at, last_error, consecutive_errors)
VALUES ($1, $2, $3, 0)
ON CONFLICT(provider) DO UPDATE SET
  last_error_at = EXCLUDED.last_error_at,
  last_error = EXCLUDED.last_error
`, ev.Provider, ev.TS.UTC(), truncate(ev.ErrorType, 200))
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
WHERE day = $1::date AND provider = $2 AND model = $3 AND pool_id = $4
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
WHERE day = $1::date AND pool_id = $2
`, day, poolID).Scan(&n)
	return n, err
}

type ProviderHealth struct {
	Provider           string
	LastSuccessAt      *time.Time
	LastErrorAt        *time.Time
	LastError          string
	ConsecutiveErrors  int64
}

func (s *Store) GetProviderHealth(ctx context.Context, provider string) (ProviderHealth, bool, error) {
	if s == nil || provider == "" {
		return ProviderHealth{}, false, nil
	}
	var h ProviderHealth
	var lastOK, lastErr sql.NullTime
	var lastErrMsg sql.NullString
	err := s.db.QueryRowContext(ctx, `
SELECT provider, last_success_at, last_error_at, last_error, consecutive_errors
FROM provider_health WHERE provider = $1
`, provider).Scan(&h.Provider, &lastOK, &lastErr, &lastErrMsg, &h.ConsecutiveErrors)
	if err == sql.ErrNoRows {
		return ProviderHealth{}, false, nil
	}
	if err != nil {
		return ProviderHealth{}, false, err
	}
	if lastOK.Valid {
		t := lastOK.Time.UTC()
		h.LastSuccessAt = &t
	}
	if lastErr.Valid {
		t := lastErr.Time.UTC()
		h.LastErrorAt = &t
	}
	if lastErrMsg.Valid {
		h.LastError = lastErrMsg.String
	}
	return h, true, nil
}

func (s *Store) Summary(ctx context.Context, fromDay, toDay, provider string) (Summary, error) {
	out := Summary{From: fromDay, To: toDay}
	if s == nil {
		return out, nil
	}

	where := `day >= $1::date AND day <= $2::date`
	args := []any{fromDay, toDay}
	if provider != "" {
		where += ` AND provider = $3`
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

// affectsCircuit reports whether a failed attempt should open/advance the
// provider circuit breaker. Quota and client/model errors must not disable
// an otherwise healthy provider.
func affectsCircuit(ev UsageEvent) bool {
	if ev.StatusCode == 429 || ev.ErrorType == "rate_limit" {
		return false
	}
	if ev.StatusCode == 400 || ev.ErrorType == "bad_request" {
		return false
	}
	if ev.StatusCode == 404 || ev.ErrorType == "not_found" {
		return false
	}
	if ev.StatusCode == 402 || ev.ErrorType == "credits" {
		return false
	}
	// Timeouts and 5xx / unknown upstream failures do.
	return true
}
