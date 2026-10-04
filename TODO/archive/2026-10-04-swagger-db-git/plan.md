# Plan: swagger-db-git

- **ID:** `2026-10-04T192600-swagger-db-git`
- **Created:** 2026-10-04T19:26:00+03:00
- **Status:** in_progress

## Goal

1. Инициализировать git remote и запушить `main` на GitHub.
2. Добавить качественный OpenAPI/Swagger UI на `/docs`.
3. Ввести SQLite как источник правды для **счётчиков квот** и **операционной статистики** (не «игрушечный» in-memory счётчик).

## Context

Прокси уже агрегирует Gemini / Groq / OpenRouter и отдаёт обогащённый `/v1/models` + `/v1/providers`.  
OpenRouter даёт live `remaining_rpd`; Gemini/Groq — только static catalog.  
Основной системе нужен предсказуемый остаток и история для поддержки.

## Design decisions

### Git

- Remote: `git@github.com:DaniilPodruchnyMgtuIu6/llm-proxy.git`
- Branch: `main`
- `.env` и `data/*.db` — в `.gitignore`, секреты не пушим.

### Swagger

- OpenAPI 3.0 YAML как source of truth: `api/openapi.yaml`
- UI: Swagger UI на `GET /docs` (и `/docs/`)
- Спека: `GET /openapi.yaml`
- Описать все ручки, схемы Model/Quota/Provider, query-параметры, ошибки.

### Database (SQLite)

- Движок: `modernc.org/sqlite` (pure Go, без CGO на Windows).
- Файл: `DATA_DIR` / `llm-proxy.db` (default `./data/llm-proxy.db`).
- Зачем БД, а не память:
  - переживает рестарты;
  - общая статистика для поддержки;
  - единый remaining для Gemini/Groq между инстансами (пока single-node).

#### Tables

**`usage_events`** — append-only лог каждого chat-вызова:

| column | notes |
|--------|--------|
| id | PK |
| ts | RFC3339 UTC |
| provider, model, pool_id | маршрутизация |
| status_code, latency_ms | ops |
| prompt/completion/total_tokens | если есть в ответе |
| error_type | `rate_limit` / `upstream` / … |
| request_id | опционально |

**`daily_counters`** — агрегаты за день (для квот):

| PK | `(day, provider, model, pool_id)` |
|----|-------------------------------------|
| day | календарный день в TZ сброса источника |
| success_count / error_count / rate_limit_count | |
| tokens_in / tokens_out | |

**`provider_health`** — last success/error для `/v1/providers`.

#### Quota merge

```
effective_remaining_rpd =
  if live_remaining != null → live   # OpenRouter
  else if catalog.rpd != null → max(0, catalog.rpd - local_success_today)
  else → null
```

`quota.source` становится `live` | `local_db` | `static_catalog`.

Timezone day bucket:

- openrouter → UTC
- gemini → America/Los_Angeles
- groq → UTC (локальный суточный учёт; upstream sliding window отдельно)

### New API (stats)

- `GET /v1/stats/summary?from=&to=&provider=` — агрегированная статистика для основной системы / поддержки.

## Out of scope (этот change)

- Auto-fallback при 429 (следующий change).
- Postgres / multi-node.
- Streaming chat.

## Risks

- SSH push может потребовать ключ/доступ к GitHub.
- Оценка Gemini RPD из каталога ≠ реальный AI Studio — `confidence=estimate` остаётся.
- Локальный счётчик для Groq приближает RPD, но не TPM/RPM sliding windows.
