CREATE TABLE IF NOT EXISTS usage_events (
  id BIGSERIAL PRIMARY KEY,
  ts TIMESTAMPTZ NOT NULL,
  provider TEXT NOT NULL,
  model TEXT NOT NULL,
  pool_id TEXT NOT NULL DEFAULT '',
  status_code INTEGER NOT NULL,
  latency_ms BIGINT NOT NULL DEFAULT 0,
  prompt_tokens BIGINT NOT NULL DEFAULT 0,
  completion_tokens BIGINT NOT NULL DEFAULT 0,
  total_tokens BIGINT NOT NULL DEFAULT 0,
  error_type TEXT NOT NULL DEFAULT '',
  request_id TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_usage_events_ts ON usage_events(ts);
CREATE INDEX IF NOT EXISTS idx_usage_events_provider ON usage_events(provider, ts);

CREATE TABLE IF NOT EXISTS daily_counters (
  day DATE NOT NULL,
  provider TEXT NOT NULL,
  model TEXT NOT NULL,
  pool_id TEXT NOT NULL DEFAULT '',
  success_count BIGINT NOT NULL DEFAULT 0,
  error_count BIGINT NOT NULL DEFAULT 0,
  rate_limit_count BIGINT NOT NULL DEFAULT 0,
  tokens_in BIGINT NOT NULL DEFAULT 0,
  tokens_out BIGINT NOT NULL DEFAULT 0,
  PRIMARY KEY (day, provider, model, pool_id)
);

CREATE TABLE IF NOT EXISTS provider_health (
  provider TEXT PRIMARY KEY,
  last_success_at TIMESTAMPTZ,
  last_error_at TIMESTAMPTZ,
  last_error TEXT,
  consecutive_errors BIGINT NOT NULL DEFAULT 0
);
