# Архитектура LLM Proxy

Документ описывает, как устроена система: компоненты, потоки данных, квоты, ранжирование моделей и учёт статистики.

> **Как смотреть диаграммы:** встроенный Markdown Preview в Cursor/VS Code **не рисует** Mermaid.  
> Открой рендер с Mermaid.js: [http://localhost:8080/architecture](http://localhost:8080/architecture)  
> На GitHub диаграммы тоже отображаются. Сырой файл: [`architecture.md`](./architecture.md).

Интерактивный API: [/docs](http://localhost:8080/docs) · карточки провайдеров: [`docs/providers/`](./providers/).

## 1. Обзор

Прокси агрегирует бесплатные LLM-источники (Gemini, Groq, OpenRouter) за единым OpenAI-совместимым API. Основная система:

1. смотрит `/v1/providers` и `/v1/models` (уже отсортированы по «мозгам»);
2. выбирает модель с учётом `rank`, `quality_score`, `quota.remaining_rpd`;
3. шлёт `/v1/chat/completions`;
4. при необходимости смотрит `/v1/stats/summary`.

```mermaid
flowchart LR
  Main[Main system] --> API[LLM Proxy :8080]
  API --> G[Gemini]
  API --> Q[Groq]
  API --> O[OpenRouter]
  API --> PG[(PostgreSQL)]
  Admin[pgAdmin :5050] --> PG
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
  subgraph Proxy[cmd/server]
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
  participant K as OpenRouter Key API

  C->>A: GET /v1/models
  A->>R: ListModels filter
  loop each provider
    R->>P: ListModels
    alt upstream OK
      P-->>R: raw models
      R->>R: ranking.Score
      alt openrouter free pool
        P->>K: GET /key
        K-->>P: remaining RPD
      else gemini or groq
        R->>S: success_count
        S-->>R: used
        R->>R: remaining = rpd - used
      end
    else upstream error
      P-->>R: skip provider
    end
  end
  R->>R: sort by score DESC
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

  C->>A: POST /v1/chat/completions
  A->>R: ChatCompletions
  R->>R: resolve provider by model
  R->>U: forward request
  U-->>R: response or error
  R-->>A: body status provider model
  A->>S: RecordUsage
  A-->>C: JSON response
```

## 5. Алгоритм ранжирования моделей

Цель: в `/v1/models` первыми идут **более сильные** chat-модели, specialty (TTS/STT/image/guard) — ниже.

Реализация: `internal/ranking.Score(provider, modelID) → int`.

```mermaid
flowchart TD
  A[model id + provider] --> B{provider?}
  B -->|gemini| G[Tier Pro / Flash / Flash-Lite]
  G --> GV[Add version bonus]
  GV --> GS[Specialty lower score]
  B -->|groq| Q[Fixed table 120b then 20b then qwen]
  B -->|openrouter| O[Heuristics by name size]
  O --> OR{openrouter/free?}
  OR -->|yes| M[Mid score auto router]
  OR -->|no| H[name heuristics]
  GS --> OUT[quality_score]
  Q --> OUT
  H --> OUT
  M --> OUT
  OUT --> SORT[stable sort DESC to rank]
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
  Start[Quota from catalog or live] --> Live{live remaining set?}
  Live -->|yes OpenRouter| Keep[Keep upstream remaining]
  Live -->|no| HasRPD{catalog rpd known?}
  HasRPD -->|no| Unk[remaining null]
  HasRPD -->|yes| DB[Postgres daily_counters]
  DB --> Calc[remaining = max 0 rpd - used]
  Calc --> Local[source local_db]
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
  P -->|gemini| PT[Pacific calendar day]
  P -->|groq or openrouter| UTC[UTC calendar day]
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
  Class -->|2xx| OK[success_count +1]
  Class -->|429| RL[rate_limit and error +1]
  Class -->|other| ER[error_count +1]
  OK --> T[tokens if present]
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
  A[GET providers] --> B[drop unhealthy]
  B --> C[GET models recommended]
  C --> D[sorted rank 1 is best]
  D --> E{remaining_rpd is 0?}
  E -->|yes| F[next model]
  E -->|no or null| G[POST chat]
  G --> H{429 or 5xx?}
  H -->|yes| F
  H -->|no| I[done]
```

Важно: для `pool_id=openrouter:free` не суммируй RPD по моделям — это **один** бюджет.

## 9. Инфраструктура данных

```mermaid
flowchart LR
  App[llm-proxy] -->|DB_* env| PG[(postgres 16)]
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
