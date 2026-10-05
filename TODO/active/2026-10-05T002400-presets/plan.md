# Plan: presets

- **ID:** `2026-10-05T002400-presets`
- **Created:** 2026-10-05T00:24:00+03:00
- **Status:** in progress (implementation done, smoke pending)
- **Branch:** `feat/presets` (from `main`)
- **Supersedes archive:** `archive/2026-10-05-gateway-ui`

## Decisions (утверждено)

| # | Тема | Выбор |
|---|------|--------|
| 1 | Endpoint | `POST /v1/p/{slug}/chat/completions` |
| 2 | Хранение | PostgreSQL |
| 3 | Override | пресет = defaults; явное поле в теле побеждает |
| 4 | Глобальные defaults UI | системный пресет `default` (`is_system`, нельзя удалить) |
| 5 | Plain `/v1/chat/completions` | подмешивает `default`; в доках главный путь — preset URL |
| 6 | Подход | минимальный: таблица + admin CRUD + UI + merge |

## Goal

Готовить пресеты (model + sampling) и давать **готовую ссылку**, чтобы основная система звала API почти только с `messages`, без ручной передачи параметров.

## Design

### Schema

```text
presets(
  slug TEXT PRIMARY KEY,          -- ^[a-z0-9][a-z0-9_-]{0,63}$
  title TEXT NOT NULL,
  description TEXT,
  model TEXT NOT NULL,            -- "auto" или model id
  temperature FLOAT8 NULL,
  top_p FLOAT8 NULL,
  top_k INT NULL,
  max_tokens INT NULL,
  presence_penalty FLOAT8 NULL,
  frequency_penalty FLOAT8 NULL,
  is_system BOOL NOT NULL DEFAULT false,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL
)
```

Миграция: создать `default` из runtime `defaults` (если есть), иначе `model=auto`.  
Usage: опционально писать `preset` slug в usage_events (nullable) — если дешево в той же задаче.

### API

| Метод | Путь | Назначение |
|-------|------|------------|
| GET | `/admin/presets` | список |
| POST | `/admin/presets` | создать |
| GET | `/admin/presets/{slug}` | один + готовый URL / curl |
| PUT | `/admin/presets/{slug}` | обновить |
| DELETE | `/admin/presets/{slug}` | удалить (запрет для `is_system`) |
| POST | `/v1/p/{slug}/chat/completions` | chat + merge пресета |
| POST | `/v1/chat/completions` | как сейчас + merge `default` |

Merge: для `model` и sampling-полей — если в body нет / null → из пресета; иначе body.  
`messages` обязательны. Валидация диапазонов — существующая `ValidateChatBody` после merge.

### UI

- Вкладка **Пресеты**: список, создать/редактировать, копировать URL и curl.
- Кнопка сохранения только на этой вкладке («Сохранить пресет» / «Создать пресет»), не глобальный header.
- **Настройки**: API base, PROXY_API_KEY, hint про пресеты (без блока defaults).
- **Чат**: выбор пресета → вызов `/v1/p/{slug}/chat/completions`.

### Docs

- `docs/api.md`, `api/openapi.yaml`, `docs/architecture.md`, README — главный путь preset URL.

## Out of scope (v1)

- Per-preset API keys
- Streaming
- Версии/история пресетов
- Multi-user ACL
- Алиасы `/v1/p/{slug}/completion` (можно позже)

## Success

1. Создал пресет `work` → получил URL → `curl` только с `messages` работает.
2. Поле в теле переопределяет пресет.
3. `default` нельзя удалить; plain chat completions его подмешивает.
4. Smoke + docs/swagger.
