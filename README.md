# LLM Proxy

HTTP-прокси для работы с разными LLM через единый OpenAI-совместимый API. Цель — агрегировать **бесплатные** источники моделей (лимиты на день/неделю) и прозрачно маршрутизировать запросы.

## Идея

Многие провайдеры дают бесплатный доступ к моделям с квотами (RPM / TPM / RPD). Вместо ручного переключения между ними прокси:

1. держит список доступных моделей и источников;
2. принимает запрос клиента в едином формате;
3. выбирает подходящий бэкенд и проксирует вызов;
4. возвращает ответ в едином формате.

## API

- Контракт: **[`docs/api.md`](./docs/api.md)**
- Swagger UI: **[/docs](http://localhost:8080/docs)** (`/openapi.yaml`)
- Планы изменений (SDD): **[`TODO/`](./TODO/)**

| Метод | Путь | Назначение |
|-------|------|------------|
| `GET` | `/docs` | Swagger UI |
| `GET` | `/openapi.yaml` | OpenAPI 3 спецификация |
| `GET` | `/healthz` | Healthcheck |
| `GET` | `/v1/providers` | Источники + квоты/здоровье |
| `GET` | `/v1/models` | Модели с `source` + `quota` |
| `GET` | `/v1/stats/summary` | Статистика из SQLite |
| `POST` | `/v1/chat/completions` | Чат (OpenAI-совместимое тело) |

Пример:

```bash
curl http://localhost:8080/v1/providers
curl "http://localhost:8080/v1/models?recommended=true"
curl "http://localhost:8080/v1/stats/summary"

curl http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gemini-3.5-flash-lite",
    "messages": [{"role":"user","content":"ping"}]
  }'
```

## Провайдеры

Карточки с лимитами, моделями и способом доступа — в **[`docs/providers/`](./docs/providers/)**.

| Провайдер | Статус | Документация | Env |
|-----------|--------|--------------|-----|
| Google Gemini (AI Studio) | подключено | [docs/providers/gemini.md](./docs/providers/gemini.md) | `GEMINI_API_KEY` |
| Groq (GroqCloud) | подключено | [docs/providers/groq.md](./docs/providers/groq.md) | `GROQ_API_KEY` |
| OpenRouter | подключено | [docs/providers/openrouter.md](./docs/providers/openrouter.md) | `OPENROUTER_API_KEY` |

Новый источник: копируй [`docs/providers/_TEMPLATE.md`](./docs/providers/_TEMPLATE.md) → заполни → подключаем код.

## Конфиг

```env
GEMINI_API_KEY=your_key_here
GROQ_API_KEY=your_key_here
OPENROUTER_API_KEY=your_key_here
ADDR=:8080
DATABASE_PATH=data/llm-proxy.db
```

## Стек

- Go (`cmd/server`)
- SQLite (`modernc.org/sqlite`) — квоты + статистика
- Провайдеры: `internal/provider/<name>`
- OpenAPI: `api/openapi.yaml`

## Запуск

```bash
go run ./cmd/server
```

## Статус

- [x] Идея зафиксирована
- [x] Проект инициализирован
- [x] Gemini (AI Studio) — list models + chat completions
- [x] Groq — list models + chat completions
- [x] OpenRouter — free models + chat completions
- [x] Обогащённый `/v1/models` + `/v1/providers` (source/quota)
- [x] Swagger UI `/docs` + OpenAPI
- [x] SQLite: usage events, daily counters, stats API
- [ ] Auto-fallback при 429
