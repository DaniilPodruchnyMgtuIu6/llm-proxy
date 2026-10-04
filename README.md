# LLM Proxy

HTTP-прокси для работы с разными LLM через единый OpenAI-совместимый API. Цель — агрегировать **бесплатные** источники моделей (лимиты на день/неделю) и прозрачно маршрутизировать запросы.

## Документация

| Документ | Содержание |
|----------|------------|
| [`docs/architecture.md`](./docs/architecture.md) | Архитектура + Mermaid (смотреть через [/architecture](http://localhost:8080/architecture)) |
| [`docs/api.md`](./docs/api.md) | Контракт для основной системы |
| [`docs/providers/`](./docs/providers/) | Карточки Gemini / Groq / OpenRouter |
| [/docs](http://localhost:8080/docs) | Swagger UI |
| [`TODO/`](./TODO/) | SDD-планы изменений |
| [`AGENTS.md`](./AGENTS.md) | Правила для AI-агента (без самовольного push) |

## API

| Метод | Путь | Назначение |
|-------|------|------------|
| `GET` | `/docs` | Swagger UI |
| `GET` | `/architecture` | Архитектура с отрисовкой Mermaid |
| `GET` | `/openapi.yaml` | OpenAPI 3 |
| `GET` | `/healthz` | Healthcheck |
| `GET` | `/v1/providers` | Источники + квоты |
| `GET` | `/v1/models` | Модели: `source`, `quota`, `quality_score`, `rank` (лучшие первые) |
| `GET` | `/v1/stats/summary` | Статистика из PostgreSQL |
| `POST` | `/v1/chat/completions` | Чат |

```bash
curl "http://localhost:8080/v1/models?recommended=true"
# data[0] — самая «мозговая» из выдачи (rank=1)
```

## Провайдеры

| Провайдер | Документация | Env |
|-----------|--------------|-----|
| Google Gemini | [gemini.md](./docs/providers/gemini.md) | `GEMINI_API_KEY` |
| Groq | [groq.md](./docs/providers/groq.md) | `GROQ_API_KEY` |
| OpenRouter | [openrouter.md](./docs/providers/openrouter.md) | `OPENROUTER_API_KEY` |

## Инфраструктура

```bash
docker compose up -d
```

| Сервис | URL / порт |
|--------|------------|
| Postgres | `localhost:5432` (user/pass/db: `llmproxy`) |
| pgAdmin | http://localhost:5050 (`admin@llm-proxy.local` / `admin`) |
| Proxy | http://localhost:8080 |

В pgAdmin: Add Server → Host `postgres` (из контейнера) или `host.docker.internal`/`localhost` с хоста, User/Password/DB `llmproxy`.

## Конфиг

См. [`.env.example`](./.env.example).

```env
ADDR=:8080
DB_HOST=localhost
DB_PORT=5432
DB_USER=llmproxy
DB_PASSWORD=llmproxy
DB_NAME=llmproxy
DB_SSLMODE=disable
GEMINI_API_KEY=...
GROQ_API_KEY=...
OPENROUTER_API_KEY=...
```

## Запуск

```bash
docker compose up -d
go run ./cmd/server
```

## Стек

- Go · PostgreSQL (`pgx`) · Docker Compose · OpenAPI/Swagger

## Статус

- [x] Multi-provider proxy (Gemini, Groq, OpenRouter)
- [x] Quotas + ranking + PostgreSQL stats
- [x] Swagger + architecture docs (Mermaid)
- [ ] Auto-fallback при 429
