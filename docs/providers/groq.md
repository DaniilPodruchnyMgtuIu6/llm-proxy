# Provider: Groq (GroqCloud)

> Статус в прокси: **подключено**  
> Последняя проверка: **2026-10-04**  
> Не путать с xAI **Grok** — это именно [Groq](https://console.groq.com/docs/overview).

## Кратко

- Docs: [Overview](https://console.groq.com/docs/overview), [Models](https://console.groq.com/docs/models), [Rate Limits](https://console.groq.com/docs/rate-limits), [OpenAI Compatibility](https://console.groq.com/docs/openai)
- Ключ: [API Keys](https://console.groq.com/keys)
- Зачем нам: быстрый OpenAI-совместимый inference с постоянным Free Plan (без карты) и понятными RPM/RPD/TPM/TPD.
- Код: `internal/provider/groq/`
- Env: `GROQ_API_KEY`

## Доступ

| Поле | Значение |
|------|----------|
| Регистрация | аккаунт на console.groq.com |
| Ключ | `gsk_...` из Console → API Keys |
| Billing для Free | не обязателен |
| Хранение | `.env` → `GROQ_API_KEY=...` |
| Опционально | `GROQ_BASE_URL` (default ниже) |

Лимиты считаются на **organization**, не на отдельный ключ. Точные цифры для своего аккаунта — в Console → Limits.

## Способ взаимодействия

| | |
|--|--|
| Base URL | `https://api.groq.com/openai/v1` |
| Auth | `Authorization: Bearer $GROQ_API_KEY` |
| List models | `GET {base}/models` |
| Chat | `POST {base}/chat/completions` |
| Docs | [OpenAI Compatibility](https://console.groq.com/docs/openai) |

Через наш прокси:

- `GET /v1/models` (модели с `"provider":"groq"`)
- `POST /v1/chat/completions` с `model` = Groq model id (например `openai/gpt-oss-20b`)

Также у Groq есть Responses API (`/responses`) — в прокси пока не используем, нам достаточно chat completions.

Неподдерживаемые поля OpenAI (дадут 400): `logprobs`, `logit_bias`, `top_logprobs`, `messages[].name`; `n` только `1`.  
Прокси **отрезает** их в `SanitizeChatBody` перед upstream (также `top_k`, `reasoning_effort`).

## Бесплатные лимиты (Free Plan)

Источник: [Rate Limits](https://console.groq.com/docs/rate-limits) (таблица free-плана; сверяй Limits в Console).

Оси:

| Метрика | Значение |
|---------|----------|
| RPM | requests per minute |
| RPD | requests per day |
| TPM | tokens per minute |
| TPD | tokens per day |
| ASH / ASD | audio seconds (для Whisper) |

Правила:

1. Лимиты **на organization**.
2. Срабатывает **первая** исчерпанная ось.
3. Cached tokens **не** считаются в rate limits.
4. При превышении → HTTP **429**; есть полезные заголовки остатка.

### Chat / text (Free Plan, по docs)

| Model ID | RPM | RPD | TPM | TPD |
|----------|-----|-----|-----|-----|
| `openai/gpt-oss-120b` | 30 | 1 000 | 8 000 | 200 000 |
| `openai/gpt-oss-20b` | 30 | 1 000 | 8 000 | 200 000 |
| `openai/gpt-oss-safeguard-20b` | 5 | 1 000 | 2 000 | 200 000 |
| `qwen/qwen3.8-27b` | 30 | 1 000 | 8 000 | 200 000 |

### Прочее на Free

| Model ID | RPM | RPD | Примечание |
|----------|-----|-----|------------|
| `whisper-large-v3` / `whisper-large-v3-turbo` | 20 | 2 000 | + ASH/ASD (аудио) |
| `canopylabs/orpheus-v1-english` / `orpheus-arabic-saudi` | 10 | 100 | TTS |
| `meta-llama/llama-prompt-guard-2-22m` / `-86m` | 30 | 14 400 | classifiers, не chat |

### Когда сбрасывается

- RPM/TPM — скользящие минутные окна (см. `x-ratelimit-reset-tokens`).
- RPD — дневное окно; остаток и время до сброса в заголовках:
  - `x-ratelimit-remaining-requests` (RPD left)
  - `x-ratelimit-reset-requests` (до сброса RPD)
  - `x-ratelimit-remaining-tokens` / `x-ratelimit-reset-tokens` (TPM)

Как понять, что кончилось: **429** + `retry-after` (секунды).

Как проверить остаток:

1. Console → Limits / usage
2. Заголовки ответа API (удобно для будущего учёта квот в прокси)

> Llama chat-модели (`llama-3.1-8b-instant`, `llama-3.3-70b-versatile` и т.п.) на Free Plan в актуальной таблице rate-limits **могут отсутствовать** (enterprise / paid). Не опирайся на старые гайды про «бесплатную Llama».

Upgrade: [Developer plan](https://console.groq.com/docs/rate-limits) — выше лимиты, Batch/Flex и т.д.

## Модели

### Рекомендуемые для free chat (проверено у нас)

| Model ID | Зачем | Проверено |
|----------|-------|-----------|
| `openai/gpt-oss-20b` | быстрый chat на free, ~1000 RPD | 2026-10-04 → ответ `ok` |
| `openai/gpt-oss-120b` | сильнее / тяжелее | в free таблице docs + в `/models` |
| `qwen/qwen3.8-27b` | альтернатива на free | в free таблице docs + в `/models` |

### Полный список с нашего ключа (2026-10-04, 11 моделей)

- chat/text: `openai/gpt-oss-20b`, `openai/gpt-oss-120b`, `openai/gpt-oss-safeguard-20b`, `qwen/qwen3.8-27b`, `allam-2-7b`
- STT: `whisper-large-v3`, `whisper-large-v3-turbo`
- TTS: `canopylabs/orpheus-v1-english`, `canopylabs/orpheus-arabic-saudi`
- classifiers: `meta-llama/llama-prompt-guard-2-22m`, `meta-llama/llama-prompt-guard-2-86m`

Обновить:

```bash
curl http://localhost:8080/v1/models
```

Прямой вызов:

```bash
curl https://api.groq.com/openai/v1/models \
  -H "Authorization: Bearer $GROQ_API_KEY"
```

## Примеры через наш прокси

```bash
curl http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "openai/gpt-oss-20b",
    "messages": [{"role":"user","content":"Reply with exactly: ok"}]
  }'
```

## Заметки / pitfalls

- Model id часто с префиксом (`openai/...`, `qwen/...`) — передавай **как есть**.
- Free chat-набор узкий: в основном GPT-OSS + Qwen 3.8; Whisper/Orpheus — отдельные модальности.
- Цены на странице Models — для paid/Developer; Free ограничен rate limits, не «вечным zero-cost enterprise Llama».
- Не коммитить `GROQ_API_KEY`.

## Ссылки

- [Overview](https://console.groq.com/docs/overview)
- [OpenAI Compatibility](https://console.groq.com/docs/openai)
- [Rate Limits](https://console.groq.com/docs/rate-limits)
- [Models](https://console.groq.com/docs/models)
