# LLM Proxy

HTTP-прокси для работы с разными LLM через единый OpenAI-совместимый API. Цель — агрегировать **бесплатные** источники моделей (лимиты на день/неделю) и прозрачно маршрутизировать запросы.

## Быстрый старт (Docker)

Нужен Docker Desktop. Ключи API **свои** — кладутся только в локальный `.env`, в образ не вшиваются.

**Windows:**

```powershell
.\scripts\start.ps1
# открой UI → вставь ключи провайдеров → Применить
```

**Linux / macOS:**

```bash
chmod +x scripts/*.sh
./scripts/start.sh
```

| Сервис | URL |
|--------|-----|
| **UI (gateway console)** | http://localhost:3000 |
| Proxy API | http://localhost:8080 |
| Swagger | http://localhost:3000/docs |
| Architecture | http://localhost:3000/architecture |
| pgAdmin | http://localhost:5050 (`admin@example.com` / `admin`) |

Ключи можно не класть в `.env`: мастер в UI пишет их в runtime volume и делает hot-reload.

Остановка: `.\scripts\stop.ps1` / `./scripts/stop.sh`.

Подробнее про раздачу друзьям и образ: [`docs/DEPLOY.md`](./docs/DEPLOY.md).

## Документация

| Документ | Содержание |
|----------|------------|
| [`docs/DEPLOY.md`](./docs/DEPLOY.md) | Docker, `.env`, передача друзьям |
| [`docs/architecture.md`](./docs/architecture.md) | Архитектура + Mermaid → [/architecture](http://localhost:8080/architecture) |
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
| `GET` | `/v1/models` | Модели: `source`, `quota`, `quality_score`, `rank` |
| `GET` | `/v1/stats/summary` | Статистика из PostgreSQL |
| `POST` | `/v1/p/{slug}/chat/completions` | **Пресет URL** (рекомендуется) |
| `POST` | `/v1/chat/completions` | Чат + merge пресета `default` |
| `POST` | `/v1/completion` | Алиас chat |
| `POST` | `/v1/route` | Авто-выбор модели |
| `GET`/`POST` | `/admin/presets` | Список / создание пресетов |

```bash
curl "http://localhost:8080/v1/models?recommended=true"
curl -s http://localhost:8080/v1/p/default/chat/completions -H "Content-Type: application/json" \
  -d '{"messages":[{"role":"user","content":"ping"}]}'
```

## Провайдеры

| Провайдер | Документация | Env |
|-----------|--------------|-----|
| Google Gemini | [gemini.md](./docs/providers/gemini.md) | `GEMINI_API_KEY` |
| Groq | [groq.md](./docs/providers/groq.md) | `GROQ_API_KEY` |
| OpenRouter | [openrouter.md](./docs/providers/openrouter.md) | `OPENROUTER_API_KEY` |

## Конфиг

См. [`.env.example`](./.env.example). Секреты только в `.env` (gitignore).

При `docker compose` приложение всегда ходит в БД на хост `postgres` (переопределение в compose), даже если в `.env` для локальной разработки указано `DB_HOST=localhost`.

## Локальная разработка без Docker-приложения

```bash
docker compose up -d postgres pgadmin   # только инфра
go run ./cmd/server                     # DB_HOST=localhost в .env
```

## Стек

- Go, OpenAI-compatible HTTP API
- PostgreSQL 16 + pgAdmin
- Docker Compose (proxy + db + pgAdmin)
