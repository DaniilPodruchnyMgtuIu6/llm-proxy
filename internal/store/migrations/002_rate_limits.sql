CREATE TABLE IF NOT EXISTS rate_limits (
  provider TEXT NOT NULL,
  model TEXT NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL,
  limit_requests BIGINT,
  remaining_requests BIGINT,
  reset_requests TEXT NOT NULL DEFAULT '',
  limit_tokens BIGINT,
  remaining_tokens BIGINT,
  reset_tokens TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (provider, model)
);
