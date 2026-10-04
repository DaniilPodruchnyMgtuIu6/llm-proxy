# API контракт для основной системы

База: `http://localhost:8080`

Интерактивно: [/docs](http://localhost:8080/docs) · спека: [/openapi.yaml](http://localhost:8080/openapi.yaml)  
Архитектура и алгоритмы (Mermaid): [`architecture.md`](./architecture.md)

Цель: основная система может **выбирать модель и источник**, зная лимиты, остаток запросов и **качество модели** (`rank` / `quality_score`), плюс смотреть статистику поддержки.

## Эндпоинты

| Метод | Путь | Назначение |
|-------|------|------------|
| `GET` | `/docs` | Swagger UI |
| `GET` | `/openapi.yaml` | OpenAPI 3 |
| `GET` | `/healthz` | liveness |
| `GET` | `/v1/providers` | источники + их квоты/здоровье |
| `GET` | `/v1/models` | модели с `source` + `quota` + `quality_score`/`rank` (лучшие первые) |
| `GET` | `/v1/stats/summary` | агрегаты из PostgreSQL |
| `POST` | `/v1/chat/completions` | OpenAI-совместимый чат |
## GET `/v1/providers`

Обзор источников для дашборда / роутера.

```json
{
  "object": "list",
  "data": [
    {
      "id": "openrouter",
      "name": "OpenRouter",
      "enabled": true,
      "healthy": true,
      "models": 22,
      "free_models": 22,
      "docs": "https://openrouter.ai/docs/quickstart",
      "quota": {
        "rpm": 20,
        "rpd": 50,
        "remaining_rpd": 48,
        "used_rpd": 2,
        "scope": "shared_pool",
        "pool_id": "openrouter:free",
        "source": "live",
        "confidence": "exact",
        "tier": "free",
        "reset_rpd_hint": "midnight_utc",
        "resets_at": "2026-10-05T00:00:00Z"
      }
    }
  ]
}
```

## GET `/v1/models`

### Query

| Параметр | Пример | Эффект |
|----------|--------|--------|
| `provider` | `groq` | только модели источника |
| `free` | `true` | только `free: true` |
| `recommended` | `true` | только рекомендованные для free-роутинга |

Примеры:

```bash
curl "http://localhost:8080/v1/models"
curl "http://localhost:8080/v1/models?provider=openrouter"
curl "http://localhost:8080/v1/models?recommended=true"
curl "http://localhost:8080/v1/models?provider=groq&free=true"
```

### Элемент модели

```json
{
  "id": "openai/gpt-oss-20b",
  "object": "model",
  "owned_by": "OpenAI",
  "provider": "groq",
  "source": { "id": "groq", "name": "Groq (GroqCloud)" },
  "free": true,
  "recommended": true,
  "quality_score": 7400,
  "rank": 3,
  "quota": {
    "rpm": 30,
    "rpd": 1000,
    "tpm": 8000,
    "tpd": 200000,
    "remaining_rpm": null,
    "remaining_rpd": 999,
    "scope": "per_model",
    "source": "local_db",
    "confidence": "exact",
    "tier": "free",
    "reset_rpd_hint": "sliding_window",
    "notes": "..."
  }
}
```

Список **уже отсортирован**: `rank=1` — самая сильная модель в текущей выдаче. Алгоритм: [`architecture.md` §5](./architecture.md).

### Поля `quota` (как читать в основной системе)

| Поле | Смысл |
|------|--------|
| `rpm` / `rpd` / `tpm` / `tpd` | потолки (null = неизвестно) |
| `remaining_rpd` / `remaining_rpm` | остаток (null = не трекаем live) |
| `used_rpd` | уже потрачено за день (если есть) |
| `scope` | `per_model` — свой счётчик; `shared_pool` — общий на группу |
| `pool_id` | id пула, напр. `openrouter:free` — модели с одним pool_id делят RPD |
| `source` | `live` / `static_catalog` / `unknown` |
| `confidence` | `exact` / `estimate` / `unknown` |
| `reset_rpd_hint` | `midnight_utc` / `midnight_pacific` / `sliding_window` |
| `resets_at` | RFC3339 следующего сброса RPD, если считаем |

### Что live / local_db / estimate

| Источник | Лимиты | Remaining |
|----------|--------|-----------|
| OpenRouter free | live RPD из `/api/v1/key` | **live** (`remaining_rpd`) |
| Groq | каталог Free Plan | **local_db** = `rpd - success_today` |
| Gemini | оценка Free Tier | **local_db** (estimate ceiling) |

БД: PostgreSQL (`DB_HOST` / `DB_PORT` / `DB_USER` / `DB_PASSWORD` / `DB_NAME` / `DB_SSLMODE`, см. `docker-compose.yml` + pgAdmin `:5050`).  
Таблицы: `usage_events`, `daily_counters`, `provider_health`.

## GET `/v1/stats/summary`

```bash
curl "http://localhost:8080/v1/stats/summary?from=2026-09-28&to=2026-10-04&provider=groq"
```

Ответ: `total_success`, `total_errors`, `total_rate_limit`, `tokens_*`, `by_provider`, `by_model`.

## Рекомендуемая логика роутера (основная система)

1. `GET /v1/providers` — отбросить `healthy=false`.
2. `GET /v1/models?recommended=true` — уже отсортировано по «мозгам» (`rank`).
3. Идти сверху вниз, пропуская `remaining_rpd == 0`.
4. Для `scope=shared_pool` не суммировать RPD по моделям — один `pool_id` = один бюджет.
5. При `429` — следующий кандидат / другой `provider`.

## POST `/v1/chat/completions`

Без изменений: OpenAI-тело, `model` = id из `/v1/models`.
