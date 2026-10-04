package store

import (
	"context"
	"database/sql"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type RateLimit struct {
	Provider           string
	Model              string
	UpdatedAt          time.Time
	LimitRequests      *int64
	RemainingRequests  *int64
	ResetRequests      string
	LimitTokens        *int64
	RemainingTokens    *int64
	ResetTokens        string
}

func ParseRateLimitHeaders(h http.Header) RateLimit {
	if h == nil {
		return RateLimit{}
	}
	return RateLimit{
		LimitRequests:     parseIntHeader(h, "X-Ratelimit-Limit-Requests"),
		RemainingRequests: parseIntHeader(h, "X-Ratelimit-Remaining-Requests"),
		ResetRequests:     firstHeader(h, "X-Ratelimit-Reset-Requests"),
		LimitTokens:       parseIntHeader(h, "X-Ratelimit-Limit-Tokens"),
		RemainingTokens:   parseIntHeader(h, "X-Ratelimit-Remaining-Tokens"),
		ResetTokens:       firstHeader(h, "X-Ratelimit-Reset-Tokens"),
	}
}

func (s *Store) UpsertRateLimit(ctx context.Context, rl RateLimit) error {
	if s == nil || rl.Provider == "" || rl.Model == "" {
		return nil
	}
	if rl.UpdatedAt.IsZero() {
		rl.UpdatedAt = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO rate_limits(
  provider, model, updated_at,
  limit_requests, remaining_requests, reset_requests,
  limit_tokens, remaining_tokens, reset_tokens
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
ON CONFLICT(provider, model) DO UPDATE SET
  updated_at = EXCLUDED.updated_at,
  limit_requests = COALESCE(EXCLUDED.limit_requests, rate_limits.limit_requests),
  remaining_requests = COALESCE(EXCLUDED.remaining_requests, rate_limits.remaining_requests),
  reset_requests = CASE WHEN EXCLUDED.reset_requests = '' THEN rate_limits.reset_requests ELSE EXCLUDED.reset_requests END,
  limit_tokens = COALESCE(EXCLUDED.limit_tokens, rate_limits.limit_tokens),
  remaining_tokens = COALESCE(EXCLUDED.remaining_tokens, rate_limits.remaining_tokens),
  reset_tokens = CASE WHEN EXCLUDED.reset_tokens = '' THEN rate_limits.reset_tokens ELSE EXCLUDED.reset_tokens END
`, rl.Provider, rl.Model, rl.UpdatedAt,
		rl.LimitRequests, rl.RemainingRequests, rl.ResetRequests,
		rl.LimitTokens, rl.RemainingTokens, rl.ResetTokens,
	)
	return err
}

func (s *Store) GetRateLimit(ctx context.Context, provider, model string) (RateLimit, bool, error) {
	if s == nil {
		return RateLimit{}, false, nil
	}
	var rl RateLimit
	var limReq, remReq, limTok, remTok sql.NullInt64
	err := s.db.QueryRowContext(ctx, `
SELECT provider, model, updated_at,
       limit_requests, remaining_requests, reset_requests,
       limit_tokens, remaining_tokens, reset_tokens
FROM rate_limits WHERE provider = $1 AND model = $2
`, provider, model).Scan(
		&rl.Provider, &rl.Model, &rl.UpdatedAt,
		&limReq, &remReq, &rl.ResetRequests,
		&limTok, &remTok, &rl.ResetTokens,
	)
	if err == sql.ErrNoRows {
		return RateLimit{}, false, nil
	}
	if err != nil {
		return RateLimit{}, false, err
	}
	rl.LimitRequests = nullInt64(limReq)
	rl.RemainingRequests = nullInt64(remReq)
	rl.LimitTokens = nullInt64(limTok)
	rl.RemainingTokens = nullInt64(remTok)
	return rl, true, nil
}

func parseIntHeader(h http.Header, key string) *int64 {
	v := firstHeader(h, key)
	if v == "" {
		return nil
	}
	// Groq sometimes returns floats in reset; limits are ints.
	if i, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64); err == nil {
		return &i
	}
	if f, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
		i := int64(f)
		return &i
	}
	return nil
}

func firstHeader(h http.Header, key string) string {
	if v := h.Get(key); v != "" {
		return v
	}
	// case-insensitive fallback
	for k, vals := range h {
		if strings.EqualFold(k, key) && len(vals) > 0 {
			return vals[0]
		}
	}
	return ""
}

func nullInt64(v sql.NullInt64) *int64 {
	if !v.Valid {
		return nil
	}
	x := v.Int64
	return &x
}
