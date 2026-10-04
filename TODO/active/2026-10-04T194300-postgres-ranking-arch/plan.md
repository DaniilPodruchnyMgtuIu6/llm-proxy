# Plan: postgres-ranking-arch

- **ID:** `2026-10-04T194300-postgres-ranking-arch`
- **Created:** 2026-10-04T19:43:00+03:00
- **Status:** in_progress

## Goal

1. Перейти с SQLite на **PostgreSQL** (Docker Compose + pgAdmin).
2. Сортировать модели **от сильных к слабым** (для приоритетного выбора основной системой).
3. Задокументировать архитектуру и алгоритмы (квоты, выбор моделей, учёт usage) со **схемами Mermaid**.
4. Обновить README, `docs/*`, OpenAPI/Swagger.

## Design

### Infra

- `docker-compose.yml`: `postgres:16-alpine` + `pgadmin4`
- Env: отдельные `DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASSWORD`, `DB_NAME`, `DB_SSLMODE` (DSN собирается в config)
- Том для данных Postgres; pgAdmin на `:5050`

### Store

- Заменить `modernc.org/sqlite` → `github.com/jackc/pgx/v5` (+ `database/sql` или pgxpool)
- Та же схема: `usage_events`, `daily_counters`, `provider_health`
- Миграции: простой SQL bootstrap при старте (как сейчас)

### Ranking

Каталог `internal/ranking` с score (выше = умнее/предпочтительнее):

| Источник | Принцип |
|----------|---------|
| Gemini | Pro > Flash > Flash-Lite; более новая major/minor выше; aliases (`*-latest`) чуть ниже явных версий |
| Groq | 120b > 20b > qwen; chat выше whisper/tts/guard |
| OpenRouter | `:free` / router; крупные reasoning/pro выше lite/mini; `openrouter/free` как удобный fallback (средний score) |

`GET /v1/models` отдаёт уже отсортированный список + поле `rank` / `quality_score`.

### Docs

- `docs/architecture.md` — обзор, компоненты, data flow, алгоритмы + Mermaid
- Обновить `docs/api.md`, provider cards (кратко про rank), OpenAPI schemas

## Out of scope

- Auto-fallback при 429 (следующий change)
- Kubernetes / managed Postgres
