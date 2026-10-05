CREATE TABLE IF NOT EXISTS presets (
  slug TEXT PRIMARY KEY,
  title TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  model TEXT NOT NULL,
  temperature DOUBLE PRECISION,
  top_p DOUBLE PRECISION,
  top_k INTEGER,
  max_tokens INTEGER,
  presence_penalty DOUBLE PRECISION,
  frequency_penalty DOUBLE PRECISION,
  is_system BOOLEAN NOT NULL DEFAULT FALSE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO presets (slug, title, description, model, is_system)
VALUES (
  'default',
  'Default',
  'System preset used by POST /v1/chat/completions when fields are omitted.',
  'auto',
  TRUE
)
ON CONFLICT (slug) DO NOTHING;

ALTER TABLE usage_events
  ADD COLUMN IF NOT EXISTS preset TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_usage_events_preset ON usage_events(preset, ts);
