# Архитектура LLM Proxy

Документ описывает, как устроена система: компоненты, потоки данных, квоты, ранжирование моделей и учёт статистики.

Интерактивный API: [/docs](http://localhost:8080/docs) · карточки провайдеров: [`docs/providers/`](./providers/).

## 1. Обзор

Прокси агрегирует бесплатные LLM-источники (Gemini, Groq, OpenRouter) за единым OpenAI-совместимым API. Основная система:

1. смотрит `/v1/providers` и `/v1/models` (уже отсортированы по «мозгам»);
2. выбирает модель с учётом `rank`, `quality_score`, `quota.remaining_rpd`;
3. шлёт `/v1/chat/completions`;
4. при необходимости смотрит `/v1/stats/summary`.

```mermaid
flowchart LR
  Main[Основная система] --> API[LLM Proxy :8080]
  API --> G[Gemini AI Studio]
  API --> Q[GroqCloud]
  API --> O[OpenRouter]
  API --> PG[(PostgreSQL)]
  PGAdmin[pgAdmin :5050] --> PG
```

## 2. Компоненты

| Компонент | Путь / сервис | Роль |
|-----------|---------------|------|
| HTTP API | `internal/api` | ручки, Swagger, запись usage |
| Registry | `internal/provider` | агрегация моделей, роутинг chat по `model` |
| Providers | `internal/provider/{gemini,groq,openrouter}` | upstream OpenAI-compat клиенты |
| Quota catalog | `internal/quota` | статические лимиты Free Tier |
| Ranking | `internal/ranking` | `quality_score` (выше = умнее) |
| Store | `internal/store` | PostgreSQL: events, counters, health |
| Postgres | `docker-compose` service `postgres` | persistence |
| pgAdmin | `docker-compose` service `pgadmin` | UI БД |

```mermaid
flowchart TB
  subgraph Proxy["cmd/server"]
    CFG[config.Load]
    API[api.Server]
    REG[provider.Registry]
    RANK[ranking.Score]
    QUOTA[quota catalog]
    ST[store.Store]
  end
  CFG --> API
  CFG --> ST
  API --> REG
  REG --> RANK
  REG --> QUOTA
  REG --> ST
  API --> ST
```

## 3. Data flow: list models

```mermaid
sequenceDiagram
  participant C as Client
  participant A as API
  participant R as Registry
  participant P as Providers
  participant S as Postgres
  participant OR as OpenRouter /key

  C->>A: GET /v1/models
  A->>R: ListModels(filter)
  loop each provider
    R->>P: ListModels()
    alt upstream OK
      P-->>R: raw models
      R->>R: quality_score = ranking.Score()
      alt openrouter free pool
        P->>OR: GET /key (live remaining)
        OR-->>P: free_model_daily_requests
      else gemini / groq
        R->>S: success_count(day, model|pool)
        S-->>R: used
        R->>R: remaining = rpd - used
      end
    else upstream error
      P-->>R: error (провайдер пропускается, остальные продолжают)
    end
  end
  R->>R: sort by quality_score DESC, assign rank
  R-->>A: ModelsResponse
  A-->>C: JSON
```

## 4. Data flow: chat completions

```mermaid
sequenceDiagram
  participant C as Client
  participant A as API
  participant R as Registry
  participant U as Upstream
  participant S as Postgres

  C->>A: POST /v1/chat/completions {model, messages}
  A->>R: ChatCompletions(body)
  R->>R: resolve provider by model id
  R->>U: forward OpenAI-compat request
  U-->>R: response / error
  R-->>A: body, status, provider, model
  A->>S: RecordUsage (event + daily_counters + health)
  A-->>C: upstream JSON (or proxy_error)
```

## 5. Алгоритм ранжирования моделей

Цель: в `/v1/models` первыми идут **более сильные** chat-модели, specialty (TTS/STT/image/guard) — ниже.

Реализация: `internal/ranking.Score(provider, modelID) → int`.

```mermaid
flowchart TD
  A[model id + provider] --> B{provider?}
  B -->|gemini| G[Tier: Pro 9200 / Flash 7800 / Flash-Lite 6400]
  G --> GV[+ version* : gemini-3.8 → +380]
  GV --> GS[Specialty penalties: image/live/tts/...]
  B -->|groq| Q[Fixed table: 120b > 20b > qwen > ...]
  B -->|openrouter| O[Heuristics: ultra/pro/Xb + :free bonus]
  O --> OR{openrouter/free?}
  OR -->|yes| M[Mid score 5800 - auto router]
  OR -->|no| H[name/params heuristics]
  GS --> OUT[quality_score]
  Q --> OUT
  H --> OUT
  M --> OUT
  OUT --> SORT[stable sort DESC → rank 1..N]
```

### Принципы по источникам (на основе docs провайдеров)

| Источник | Приоритет «мозгов» |
|----------|-------------------|
| Gemini | Pro ≫ Flash ≫ Flash-Lite; новее major/minor выше; `*-latest` чуть ниже явной версии |
| Groq Free | `gpt-oss-120b` > `gpt-oss-20b` ≈ `qwen3.8-27b` ≫ safeguard / whisper / orpheus / prompt-guard |
| OpenRouter free | крупные / reasoning / pro выше mini-lite; `openrouter/free` — удобный mid-priority fallback |

Поля ответа:

- `quality_score` — сырой score
- `rank` — позиция после сортировки (`1` = лучшая в выдаче)

Рекомендация основной системе: брать первый элемент с `remaining_rpd > 0` (или `null` remaining + healthy provider).

## 6. Алгоритм квот и remaining

```mermaid
flowchart TD
  Start[Model.Quota from catalog / live] --> Live{source=live AND remaining_rpd set?}
  Live -->|yes OpenRouter| Keep[Keep upstream remaining]
  Live -->|no| HasRPD{catalog.rpd known?}
  HasRPD -->|no| Unk[remaining = null]
  HasRPD -->|yes| DB[Postgres daily_counters.success_count]
  DB --> Calc["remaining = max(0, rpd - used)"]
  Calc --> Local[source = local_db]
```

| Поле | Смысл |
|------|--------|
| `scope=per_model` | свой счётчик на `(day, provider, model)` |
| `scope=shared_pool` + `pool_id` | общий бюджет (все OpenRouter free → `openrouter:free`) |
| `reset_rpd_hint` | `midnight_pacific` (Gemini), `midnight_utc` (OpenRouter), `sliding_window` (Groq docs) |
| `confidence` | `exact` / `estimate` / `unknown` |

### Дневной bucket

```mermaid
flowchart LR
  Ev[usage event ts] --> P{provider}
  P -->|gemini| PT[America/Los_Angeles calendar day]
  P -->|groq / openrouter| UTC[UTC calendar day]
  PT --> Key[(daily_counters.day)]
  UTC --> Key
```

> Groq на upstream использует sliding windows; локальный RPD-счётчик — **аппроксимация** для роутера. OpenRouter live remaining — источник истины для free pool.

## 7. Алгоритм учёта usage / статистики

При каждом chat (успех или ошибка):

1. `INSERT usage_events` — append-only лог
2. `UPSERT daily_counters` — инкремент success/error/rate_limit + tokens
3. `UPSERT provider_health` — last success/error, consecutive_errors

```mermaid
flowchart TB
  Chat[Chat response] --> Class{status}
  Class -->|2xx| OK[success_count + 1]
  Class -->|429| RL[rate_limit_count + 1<br/>error_count + 1]
  Class -->|other err| ER[error_count + 1]
  OK --> T[tokens from usage if present]
  RL --> H[provider_health]
  ER --> H
  OK --> H
  T --> DC[(daily_counters)]
  H --> PH[(provider_health)]
  Chat --> UE[(usage_events)]
```

`GET /v1/stats/summary` агрегирует counters за `[from, to]` (по умолчанию 7 дней UTC).

## 8. Рекомендуемый роутинг основной системы

```mermaid
flowchart TD
  A[GET /v1/providers] --> B[drop unhealthy]
  B --> C[GET /v1/models?recommended=true]
  C --> D[уже отсортировано: rank 1 = лучшая]
  D --> E{remaining_rpd == 0?}
  E -->|yes| F[next model]
  E -->|no / null| G[POST chat]
  G --> H{429 / 5xx?}
  H -->|yes| F
  H -->|no| I[done]
```

Важно: для `pool_id=openrouter:free` не суммируй RPD по моделям — это **один** бюджет.

## 9. Инфраструктура данных

```mermaid
flowchart LR
  App[llm-proxy] -->|DB_HOST DB_PORT DB_USER DB_PASSWORD DB_NAME| PG[(postgres:16)]
  Admin[pgAdmin :5050] -->|UI| PG
```

Запуск:

```bash
docker compose up -d
# pgAdmin: http://localhost:5050
#   login: admin@llm-proxy.local / admin
#   добавить server Host=postgres User=llmproxy Password=llmproxy DB=llmproxy
```

Таблицы: см. `internal/store` migrate SQL.

## 10. Конфигурация

| Env | Default |
|-----|---------|
| `DB_HOST` | `localhost` |
| `DB_PORT` | `5432` |
| `DB_USER` | `llmproxy` |
| `DB_PASSWORD` | `llmproxy` |
| `DB_NAME` | `llmproxy` |
| `DB_SSLMODE` | `disable` |
| `ADDR` | `:8080` |
| `GEMINI_API_KEY` / `GROQ_API_KEY` / `OPENROUTER_API_KEY` | — |

DSN для pgx собирается в `config.Load()` из `DB_*`. См. `.env.example`.
