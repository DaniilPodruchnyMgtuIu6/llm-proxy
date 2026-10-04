# Plan: fallback-route-limits

- **ID:** `2026-10-04T195800-fallback-route-limits`
- **Created:** 2026-10-04T19:58:00+03:00
- **Status:** in_progress
- **Supersedes archive:** `archive/2026-10-04-postgres-ranking-arch`

## Goal

Реализовать пункты улучшений **1–3** и **7–9**:

1. **Auto-fallback** при 429 / 5xx на следующий model/provider по `rank` + remaining.
2. **`POST /v1/route`** (+ `model: "auto"` в chat) — прокси сам выбирает лучшую доступную модель.
3. **RPM/TPM учёт** — парсинг rate-limit headers (Groq и др.), отражение в quota.
7. **Integration tests** — Postgres + mock upstream.
8. **Миграции версионировать** + починить **pgAdmin**.
9. **SDD-архив** предыдущей фичи (tag + `ROLLBACK.md`) — делается при старте этого change.

## Design

### Fallback / route

```
candidates = models sorted by rank
  filter: healthy provider, remaining_rpd != 0 (if known), optional provider/recommended
try each:
  on 2xx → return (+ headers X-LLM-Proxy-Model, X-LLM-Proxy-Attempts)
  on 429/5xx → next candidate
  on 4xx (except 429) → stop or next? → next only for 429/502/503/504
if all fail → last upstream error or 503 proxy_error
```

- `POST /v1/chat/completions` with `"model":"auto"` → same router.
- `POST /v1/route` body: `{ messages, provider?, recommended_only?, exclude_models? }` → chat response.

### RPM/TPM

- Parse response headers after upstream chat:
  - `x-ratelimit-remaining-requests`, `x-ratelimit-remaining-tokens`
  - `x-ratelimit-limit-requests`, `x-ratelimit-limit-tokens`
  - `x-ratelimit-reset-requests`, `x-ratelimit-reset-tokens`
- Table `limit_snapshots` / upsert `rate_limits` per (provider, model).
- Merge into `quota.remaining_rpm` / notes when listing models (source=`live_headers` or `local_db`).

### Migrations

- `internal/store/migrations/*.sql` + table `schema_migrations`.
- Bootstrap applies pending files in order.

### pgAdmin

- Упростить env, `PGADMIN_CONFIG_MASTER_PASSWORD_REQUIRED=False`, корректный email.
- При необходимости сбросить volume / user.

### Tests

- `go test ./...` с build tag `integration` optional.
- httptest mock providers + real Postgres via `DATABASE_URL` or skip if unreachable.

## Out of scope

- Streaming SSE
- Auth на прокси
- Prometheus
