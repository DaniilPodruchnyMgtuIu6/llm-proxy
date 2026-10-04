# Provider: OpenRouter

> Статус в прокси: **подключено**  
> Последняя проверка: **2026-10-04**

## Кратко

- Docs: [Quickstart](https://openrouter.ai/docs/quickstart), [FAQ](https://openrouter.ai/docs/faq), [Limits](https://openrouter.ai/docs/api_reference/limits), [:free variant](https://openrouter.ai/docs/guides/routing/model-variants/free), [Free Models Router](https://openrouter.ai/docs/guides/routing/routers/free-router)
- Ключ: [openrouter.ai/keys](https://openrouter.ai/keys)
- Зачем нам: один API ко многим моделям + отдельные **бесплатные** варианты (`:free`) и роутер `openrouter/free`.
- Код: `internal/provider/openrouter/`
- Env: `OPENROUTER_API_KEY`

## Доступ

| Поле | Значение |
|------|----------|
| Регистрация | аккаунт на openrouter.ai |
| Ключ | `sk-or-v1-...` |
| Credits | для **бесплатных** моделей кредиты не списываются; но отрицательный баланс может блокировать даже free |
| Хранение | `.env` → `OPENROUTER_API_KEY=...` |
| Опционально | `OPENROUTER_BASE_URL`, `OPENROUTER_SITE_URL`, `OPENROUTER_SITE_TITLE` |

Опциональные заголовки (для leaderboard; мы шлём по умолчанию):

- `HTTP-Referer` ← `OPENROUTER_SITE_URL` (default `http://localhost:8080`)
- `X-OpenRouter-Title` ← `OPENROUTER_SITE_TITLE` (default `llm-proxy`)

## Способ взаимодействия

| | |
|--|--|
| Base URL | `https://openrouter.ai/api/v1` |
| Auth | `Authorization: Bearer $OPENROUTER_API_KEY` |
| List models | `GET {base}/models` |
| Chat | `POST {base}/chat/completions` |
| Ключ / квота | `GET {base}/key` → `free_model_daily_requests` |

Формат — OpenAI-compatible ([quickstart](https://openrouter.ai/docs/quickstart)).

В нашем прокси `GET /v1/models` для OpenRouter отдаёт **только бесплатные** модели (id с суффиксом `:free`, нулевая цена, либо `openrouter/free`). Paid-каталог намеренно не мешаем в общий список.

Chat форвардит основные OpenAI-поля + `top_k` / `reasoning_effort` / logprobs-семейство (если клиент прислал).  
Можно дернуть и paid model id, если на аккаунте есть кредиты. Без `model` прокси сам выберет free-кандидата по rank.

## Бесплатное использование

### Что считается free

1. Модели с суффиксом **`:free`** (отдельная catalog-variant) — [docs](https://openrouter.ai/docs/guides/routing/model-variants/free)  
   Пример: `meta-llama/llama-3.2-3b-instruct:free`
2. Роутер **`openrouter/free`** — сам выбирает свободную модель под запрос — [docs](https://openrouter.ai/docs/guides/routing/routers/free-router)

### Лимиты free-моделей (платформенные)

Источник: [API Credit & Rate Limits](https://openrouter.ai/docs/api_reference/limits) / [FAQ](https://openrouter.ai/docs/faq).

| Credits purchased (all-time) | RPM | RPD (на все `:free` суммарно) |
|------------------------------|-----|-------------------------------|
| меньше ~10 | **20** | **50** |
| хотя бы ~10 | **20** | **1000** |

Важные правила:

- Лимит **на аккаунт**, не на ключ (новые ключи/аккаунты лимит не размножают).
- Дневной счётчик **`free_model_daily_requests` сбрасывается в полночь UTC** (= 03:00 MSK).
- Upstream-провайдер free-модели тоже может отдать **429** (пиковая нагрузка) — это отдельно от платформенного RPD.
- Paid-модели: у OpenRouter нет такого же request-cap, но нужны кредиты; upstream может троттлить.

### Как проверить остаток

```bash
curl https://openrouter.ai/api/v1/key \
  -H "Authorization: Bearer $OPENROUTER_API_KEY"
```

Смотри:

- `data.free_model_daily_requests.limit` / `.used` / `.remaining`
- `data.is_free_tier`
- `data.limit_remaining` (кредиты на ключе, для paid)

### Когда кончилось

| Код | Смысл |
|-----|--------|
| **429** | rate limit (free RPD/RPM или upstream) |
| **402** | кредиты / in-flight budget (обычно paid; иногда блок при отрицательном балансе) |

## Модели

### Рекомендуемые для free

| Model ID | Зачем |
|----------|--------|
| `openrouter/free` | авто-выбор любой доступной free-модели |
| любой `*:free` из `/v1/models` | конкретная модель без стоимости |

Список free меняется часто — всегда бери актуальный из прокси:

```bash
curl http://localhost:8080/v1/models
# фильтр: "provider":"openrouter"
```

### Проверено у нас (2026-10-04)

| Model ID | Результат |
|----------|-----------|
| `openrouter/free` | chat → `ok` |
| free catalog via proxy | **22** модели (фильтр `:free` / цена 0 / роутер) |

Примеры id из списка: `qwen/qwen3.8-27b:free`, `google/gemma-4-31b-it:free`, `apodex/apodex-1.1-mini:free`, `openrouter/free`.

## Примеры через наш прокси

```bash
# авто free
curl http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "openrouter/free",
    "messages": [{"role":"user","content":"Reply with exactly: ok"}]
  }'

# конкретная free-модель (подставь id из /v1/models)
curl http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "meta-llama/llama-3.2-3b-instruct:free",
    "messages": [{"role":"user","content":"Reply with exactly: ok"}]
  }'
```

## Заметки / pitfalls

- Суффикс `:free` обязателен для бесплатного варианта; без него это обычно **paid** slug.
- Наши Gemini/Groq model id не пересекаются с OpenRouter `:free` (другие имена).
- Free availability плавает — роутер `openrouter/free` удобен как fallback.
- Не коммитить `OPENROUTER_API_KEY`.

## Ссылки

- [Quickstart](https://openrouter.ai/docs/quickstart)
- [Limits](https://openrouter.ai/docs/api_reference/limits)
- [Free variant](https://openrouter.ai/docs/guides/routing/model-variants/free)
- [Free Models Router](https://openrouter.ai/docs/guides/routing/routers/free-router)
- [FAQ (rate limits / free tier)](https://openrouter.ai/docs/faq)
