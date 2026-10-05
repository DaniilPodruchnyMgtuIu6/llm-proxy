# API контракт для основной системы

База API: `http://localhost:8080` · Gateway UI: `http://localhost:3000` (проксирует `/v1`, `/admin`, `/docs`)

Интерактивно: [/docs](http://localhost:8080/docs) · спека: [/openapi.yaml](http://localhost:8080/openapi.yaml)  
Архитектура и алгоритмы (Mermaid): [`architecture.md`](./architecture.md)

Цель: основная система может **выбирать модель и источник**, зная лимиты, остаток запросов и **качество модели** (`rank` / `quality_score`), плюс смотреть статистику поддержки. UI упрощает setup ключей и Apply без `docker compose up`.

## Эндпоинты

| Метод | Путь | Назначение |
|-------|------|------------|
| `GET` | `/docs` | Swagger UI |
| `GET` | `/openapi.yaml` | OpenAPI 3 |
| `GET` | `/healthz` | liveness |
| `GET` | `/admin/status` | setup / masked keys |
| `PUT` | `/admin/keys` | сохранить ключи в runtime volume (+ `apply`) |
| `PUT` | `/admin/defaults` | legacy: синхронизирует системный пресет `default` |
| `GET`/`POST` | `/admin/presets` | список / создать пресет |
| `GET`/`PUT`/`DELETE` | `/admin/presets/{slug}` | CRUD пресета (+ готовый URL/curl) |
| `POST` | `/admin/apply` | hot-reload провайдеров |
| `GET` | `/v1/providers` | источники + их квоты/здоровье |
| `GET` | `/v1/models` | модели с `source` + `quota` + `quality_score`/`rank` (лучшие первые) |
| `GET` | `/v1/stats/summary` | агрегаты из PostgreSQL |
| `POST` | `/v1/p/{slug}/chat/completions` | **главный путь**: chat с merge пресета |
| `POST` | `/v1/chat/completions` | chat + merge системного пресета `default` |
| `POST` | `/v1/completions` | алиас chat |
| `POST` | `/v1/completion` | алиас chat |
| `POST` | `/v1/route` | авто-выбор модели (фильтры `provider` / `recommended_only`) |

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
| `source` | `live` / `live_headers` / `local_db` / `static_catalog` / `unknown` |
| `confidence` | `exact` / `estimate` / `unknown` |
| `reset_rpd_hint` | `midnight_utc` / `midnight_pacific` / `sliding_window` |
| `resets_at` | RFC3339 следующего сброса RPD, если считаем |
| `remaining_tpm` | остаток TPM из rate-limit headers (если есть) |

### Что live / local_db / estimate

| Источник | Лимиты | Remaining |
|----------|--------|-----------|
| OpenRouter free | live RPD из `/api/v1/key` | **live** (`remaining_rpd`) |
| Groq | каталог Free Plan + headers | **local_db** RPD + **live_headers** RPM/TPM |
| Gemini | оценка Free Tier | **local_db** (estimate ceiling) |

БД: PostgreSQL (`DB_HOST` / `DB_PORT` / `DB_USER` / `DB_PASSWORD` / `DB_NAME` / `DB_SSLMODE`, см. `docker-compose.yml` + pgAdmin `:5050`).  
Таблицы: `usage_events` (поле `preset`), `daily_counters`, `provider_health`, `rate_limits`, `presets`, `schema_migrations`.

## Пресеты

Именованный конфиг `model` + sampling в PostgreSQL. Основная система вызывает готовый URL и почти всегда шлёт только `messages`.

### Главный путь

```bash
# 1) создать пресет (или через UI → Пресеты)
curl -s http://localhost:8080/admin/presets -H "Content-Type: application/json" -d '{
  "slug":"work","title":"Work","model":"auto","temperature":0.2,"max_tokens":256
}'

# 2) вызов — достаточно messages; явное поле в теле перекрывает пресет
curl -s http://localhost:8080/v1/p/work/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"messages":[{"role":"user","content":"ping"}]}'
```

Merge: если в теле нет поля / `null` / пустой `model` → берётся из пресета; иначе побеждает тело.  
`POST /v1/chat/completions` автоматически подмешивает системный пресет `default` (нельзя удалить).  
В `usage_events.preset` пишется slug; ответный заголовок `X-LLM-Proxy-Preset`.

| Метод | Путь | Заметки |
|-------|------|---------|
| `GET` | `/admin/presets` | список |
| `POST` | `/admin/presets` | создать (`slug` ^[a-z0-9][a-z0-9_-]{0,63}$) |
| `GET` | `/admin/presets/{slug}` | пресет + `url` + `curl_example` |
| `PUT` | `/admin/presets/{slug}` | обновить |
| `DELETE` | `/admin/presets/{slug}` | запрещено для `is_system` |

## GET `/v1/stats/summary`

```bash
curl "http://localhost:8080/v1/stats/summary?from=2026-09-28&to=2026-10-04&provider=groq"
```

Ответ: `total_success`, `total_errors`, `total_rate_limit`, `tokens_*`, `by_provider`, `by_model`.

## Рекомендуемая логика роутера (основная система)

Простой путь — отдать выбор прокси:

```bash
curl -s http://localhost:8080/v1/route -H "Content-Type: application/json" -d "{\"messages\":[{\"role\":\"user\",\"content\":\"ping\"}],\"recommended_only\":true}"
```

Или вручную:

1. `GET /v1/providers` — отбросить `healthy=false`.
2. `GET /v1/models?recommended=true` — уже отсортировано по «мозгам» (`rank`).
3. Идти сверху вниз, пропуская `remaining_rpd == 0` / `remaining_rpm == 0`.
4. Для `scope=shared_pool` не суммировать RPD по моделям — один `pool_id` = один бюджет.
5. При `429` — следующий кандидат (прокси сделает это сам).

## POST `/v1/p/{slug}/chat/completions` (рекомендуется)

То же OpenAI-тело, что у chat, но недостающие `model`/sampling берутся из пресета `{slug}`.

```bash
curl -s http://localhost:8080/v1/p/default/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"messages":[{"role":"user","content":"ping"}]}'
```

## POST `/v1/chat/completions`

Эквивалентно `/v1/p/default/chat/completions`: подмешивает системный пресет `default`.  
Алиасы: `/v1/completions`, `/v1/completion`.

```bash
curl -s http://localhost:8080/v1/completion -H "Content-Type: application/json" -d "{\"messages\":[{\"role\":\"user\",\"content\":\"ping\"}],\"temperature\":0.7,\"max_tokens\":64}"
```

Основные параметры (принимаются API): `temperature`, `top_p`, `top_k`, `max_tokens`, `max_completion_tokens`, `n`, `stop`, `stream`, `presence_penalty`, `frequency_penalty`, `seed`, `user`, `response_format`, `tools`, `tool_choice`, `parallel_tool_calls`, `reasoning_effort`, `modalities`, `logit_bias`, `logprobs`, `top_logprobs`.

Перед upstream прокси **отрезает** поля, которые конкретный провайдер не принимает (например Gemini: `presence_penalty`, `frequency_penalty`, `top_k`). Клиент может слать единый набор параметров.

Жёсткая валидация до роутинга (иначе `400`):

| Поле | Диапазон |
|------|----------|
| `temperature` | 0…2 |
| `top_p` | 0.0…1.0 |
| `top_k` | 1…200 |
| `max_tokens` / `max_completion_tokens` | 8…128000 |
| `presence_penalty` / `frequency_penalty` | −2…2 |
| `n` | только `1` |

Пустой ответ с `finish_reason=length` (типично при слишком маленьком `max_tokens`) → один retry с бюджетом ≥256, затем fallback.

Перед upstream тело **санитизируется** под провайдер:

| Поле | Gemini | Groq | OpenRouter |
|------|--------|------|------------|
| temperature / top_p / max_tokens | да | да | да |
| top_k | нет | нет | да |
| reasoning_effort | да | нет | да |
| logprobs / logit_bias / top_logprobs | нет | нет | да |
| messages[].name | да | нет | да |
| n ≠ 1 | да | нет (отбрасываем) | да |

При 429/502/503/504 — fallback на следующий кандидат по `rank`.  
Заголовки ответа: `X-LLM-Proxy-Model`, `X-LLM-Proxy-Provider`, `X-LLM-Proxy-Attempts`, `X-LLM-Proxy-Preset`, `X-Request-ID`.

### Request ID и логи

- Можно передать свой id: заголовок `X-Request-ID` (буквы/цифры/`-_./`, до 128 символов).
- Если не передан — прокси сгенерирует UUID и вернёт в `X-Request-ID`.
- Все строки лога содержат `req_id=...`; в PostgreSQL `usage_events.request_id` = этот же proxy id.
- Уровень: env `LOG_LEVEL=error|info|debug` (по умолчанию `info`). `debug` включает попытки роутинга/fallback и `upstream_id`.

### Auth (опционально)

Если в `.env` задан `PROXY_API_KEY`, все `/v1/*` требуют:

```http
Authorization: Bearer <PROXY_API_KEY>
```

или `X-API-Key: <PROXY_API_KEY>`.  
Без ключа (`PROXY_API_KEY` пустой) — открытый доступ как раньше.  
Публично без ключа: `/healthz`, `/docs`, `/openapi.yaml`, `/architecture`.

### Reliability

| Env | Default | Смысл |
|-----|---------|--------|
| `UPSTREAM_TIMEOUT` | `60s` | deadline одной upstream-попытки |
| `MAX_FALLBACK_ATTEMPTS` | `3` | максимум upstream-попыток; после 429 провайдер skip'ается в этом запросе |
| `MODELS_CACHE_TTL` | `45s` | кэш каталога `/v1/models` |
| `CIRCUIT_BREAKER_ERRORS` | `5` | подряд ошибок → временно skip provider |
| `CIRCUIT_BREAKER_COOLDOWN` | `5m` | сколько держать circuit open |

Кандидаты с `remaining_rpd/rpm/tpm == 0` не вызываются. Usage пишется **на каждую** attempt (включая 429).

## POST `/v1/route`

Тело как chat + опционально `provider`, `recommended_only`, `exclude_models` (не уходят upstream). `model` принудительно `auto`.

## Admin (Gateway UI)

Используются консолью на `:3000`. Ключи пишутся в runtime volume (`RUNTIME_DATA_DIR`, по умолчанию `/data`) и применяются hot-reload'ом — **без** `docker compose up`.

```bash
# статус / masked keys / defaults
curl -s http://localhost:8080/admin/status

# сохранить ключ и сразу применить
curl -s -X PUT http://localhost:8080/admin/keys \
  -H "Content-Type: application/json" \
  -d '{"gemini_api_key":"...","apply":true}'

# defaults модели/семплинга
curl -s -X PUT http://localhost:8080/admin/defaults \
  -H "Content-Type: application/json" \
  -d '{"model":"auto","temperature":0.2}'

# явный reload из runtime + env
curl -s -X POST http://localhost:8080/admin/apply
```
