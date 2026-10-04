# Provider: Google Gemini (AI Studio)

> Статус в прокси: **подключено**  
> Последняя проверка: **2026-10-04** (list models + chat `gemini-3.5-flash-lite`)

## Кратко

- Сайт: [Google AI Studio](https://aistudio.google.com/docs)
- API docs: [ai.google.dev](https://ai.google.dev/gemini-api/docs)
- Ключ: [Create API Key](https://aistudio.google.com/apikey)
- Зачем нам: бесплатный Free Tier с дневными/минутными квотами на текст-модели; OpenAI-совместимый endpoint упрощает прокси.
- Код: `internal/provider/gemini/`
- Env: `GEMINI_API_KEY` (уже в `.env`)

## Доступ

| Поле | Значение |
|------|----------|
| Регистрация | Google-аккаунт + AI Studio / Gemini API |
| Ключ | API key из AI Studio |
| Billing для Free | не обязателен |
| Хранение | `.env` → `GEMINI_API_KEY=...` |
| Опционально | `GEMINI_BASE_URL` (по умолчанию OpenAI-compat URL ниже) |

На Free Tier промпты/ответы **могут использоваться Google для улучшения продуктов**. На paid tiers — обычно нет. См. [pricing](https://ai.google.dev/gemini-api/docs/pricing).

## Способ взаимодействия

Мы ходим **не** в Interactions API напрямую, а в **OpenAI-compatible** слой Gemini:

| | |
|--|--|
| Base URL | `https://generativelanguage.googleapis.com/v1beta/openai` |
| Auth | `Authorization: Bearer $GEMINI_API_KEY` |
| List models | `GET {base}/models` |
| Chat | `POST {base}/chat/completions` |
| Docs | [OpenAI compatibility](https://ai.google.dev/gemini-api/docs/openai) |

Альтернативы (пока не используем в прокси):

- Native: `POST /v1beta/models/{model}:generateContent` + `x-goog-api-key`
- Interactions API (новый default в доках AI Studio с ~июня 2026)

Через наш прокси клиенту всё равно:

- `GET /v1/models`
- `POST /v1/chat/completions`

## Бесплатные лимиты

Официально Google считает лимиты по осям ([rate limits](https://ai.google.dev/gemini-api/docs/rate-limits)):

| Метрика | Что значит |
|---------|------------|
| **RPM** | requests per minute |
| **TPM** | tokens per minute (input) |
| **RPD** | requests per day |

Важные правила:

1. Лимиты **на Google Cloud / AI Studio project**, не на отдельный API key.
2. **RPD сбрасывается в полночь Pacific Time** (PT).  
   Для Москвы (UTC+3): обычно **10:00 MSK** зимой (PST, UTC-8) или **11:00 MSK** летом (PDT, UTC-7).
3. Превышение любой оси → ошибка, даже если другие оси свободны.
4. Точные RPM/TPM/RPD **зависят от модели и тира** — смотреть в AI Studio Rate limits. Google прямо пишет, что таблица не «гарантированная константа».

### Usage tiers (кратко)

| Tier | Как попасть |
|------|-------------|
| Free | активный проект / free trial |
| Tier 1 | привязан billing |
| Tier 2+ | накопленный spend + время |

Нам важен **Free**.

### Ориентиры Free (неофициальные, сверять в AI Studio)

Цифры из публичных обзоров на начало/середину 2026; **могут устареть**:

| Модель (семейство) | RPM (ориентир) | RPD (ориентир) | TPM (ориентир) |
|--------------------|----------------|----------------|----------------|
| Gemini 2.5 Flash-Lite | ~15 | ~1000 | ~250k |
| Gemini 2.5 Flash | ~10 | ~250 | ~250k |
| Gemini 2.5 Pro | ~5 | ~100 | ~250k |
| Gemini 3.x Flash / Flash-Lite | free input/output в pricing; RPM/RPD — в AI Studio | | |

Pricing: у многих актуальных Flash/Flash-Lite/Pro строк Input/Output на Free = **Free of charge** ([pricing](https://ai.google.dev/gemini-api/docs/pricing)).

### Как понять, что квота кончилась

- HTTP **429**, статус вида `RESOURCE_EXHAUSTED`
- Иногда в тексте есть hint про quota / rate limit
- Для устаревших model id у новых аккаунтов бывает **404** с текстом «no longer available… use models/…»

Пример (проверено 2026-10-04): `gemini-2.5-flash-lite` → 404, рекомендация перейти на `gemini-3.5-flash-lite`.

### Как проверить остаток

| Способ | Есть? |
|--------|-------|
| Отдельный API «сколько RPD осталось» | нет (насколько известно) |
| AI Studio → Rate limits / usage | да — основной источник |
| Заголовки ответа с remaining | не опираемся |

Практика: вести свой счётчик в прокси позже; пока смотри AI Studio и ошибки 429.

## Модели

### Рекомендуемые для free / текста (проверено у нас)

| Model ID | Зачем | Проверено |
|----------|-------|-----------|
| `gemini-3.5-flash-lite` | дешёвый/быстрый текст на free, chat OK | 2026-10-04 → ответ `ok` |
| `gemini-flash-lite-latest` | алиас «последний lite» | в списке API |
| `gemini-flash-latest` | алиас flash | в списке API |
| `gemini-3.8-flash` | более новая flash-линейка | в списке API (chat не гоняли) |
| `gemini-3.5-flash` | сильнее lite | в списке API |

### Не рекомендуем слепо брать старые id

| Model ID | Проблема |
|----------|----------|
| `gemini-2.5-flash-lite` | 404 для новых пользователей (переезд на 3.5) |

### Полный список, который отдал API на 2026-10-04 (61 шт.)

Текст / chat-ориентированные и алиасы:

- `gemini-2.5-flash`, `gemini-2.5-pro`, `gemini-2.5-flash-lite`
- `gemini-3-flash-preview`
- `gemini-3.1-pro-preview`, `gemini-3.1-pro-preview-customtools`
- `gemini-3.1-flash-lite`, `gemini-3.1-flash-lite-preview`
- `gemini-3.5-flash`, `gemini-3.5-flash-lite`
- `gemini-3.6-flash`, `gemini-3.7-flash`, `gemini-3.8-flash`
- `gemini-flash-latest`, `gemini-flash-lite-latest`, `gemini-pro-latest`
- `gemma-4-26b-a4b-it`, `gemma-4-31b-it`

Картинки / media / спец:

- image: `gemini-2.5-flash-image`, `gemini-3.1-flash-image`, `gemini-3.1-flash-image-preview`, `gemini-3.1-flash-lite-image`, `gemini-3-pro-image`, `gemini-3-pro-image-preview`, `nano-banana-pro-preview`
- TTS / live / audio: `gemini-2.5-flash-preview-tts`, `gemini-2.5-pro-preview-tts`, `gemini-3.1-flash-tts-preview`, `gemini-3.8-flash-tts`, `gemini-3.8-flash-lite-tts`, `gemini-3.8-live`, `gemini-3.8-live-extended-thinking`, `gemini-3.1-flash-live-preview`, `gemini-2.5-flash-native-audio-*`, `gemini-3.5-live-translate-preview`, `gemini-3.5-transcribe`, `gemini-3.5-transcribe-live`
- video: `veo-3.1-generate-preview`, `veo-3.1-fast-generate-preview`, `veo-3.1-lite-generate-preview`
- music: `lyria-3.5`, `lyria-3-clip-preview`, `lyria-3-pro-preview`, `lyria-realtime-exp`
- embeddings: `gemini-embedding-001`, `gemini-embedding-2`, `gemini-embedding-2-preview`
- agents / research / robotics / omni: `antigravity-preview-*`, `deep-research-*`, `gemini-robotics-er-2-*`, `gemini-omni-*`, `gemini-2.5-computer-use-preview-10-2025`, `aqa`

Обновить список:

```bash
curl http://localhost:8080/v1/models
```

В ответе у каждой модели есть `"provider":"gemini"`.

## Примеры через наш прокси

```bash
# список
curl http://localhost:8080/v1/models

# чат
curl http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gemini-3.5-flash-lite",
    "messages": [{"role":"user","content":"ping"}]
  }'
```

Прямой вызов Gemini (минуя прокси) — тот же OpenAI-формат на их base URL.

## Заметки / pitfalls

- Список `/models` ≠ гарантия, что model id ещё работает для chat на твоём аккаунте (см. 2.5 → 3.5).
- Preview/experimental модели часто с урезанными лимитами.
- Image/video/TTS могут не быть на free или иметь отдельные квоты (IPM и т.п.) — перед использованием сверяй pricing + AI Studio.
- Interactions API в доках AI Studio позиционируется как новый default; наш прокси пока на OpenAI-compat — этого достаточно для chat.
- Ключ не коммитить (`.env` в `.gitignore`).

## Ссылки

- [AI Studio docs](https://aistudio.google.com/docs)
- [Rate limits](https://ai.google.dev/gemini-api/docs/rate-limits)
- [Pricing](https://ai.google.dev/gemini-api/docs/pricing)
- [OpenAI compatibility](https://ai.google.dev/gemini-api/docs/openai)
- [Models API](https://ai.google.dev/api/models)
