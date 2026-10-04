# Как запустить и передать друзьям

Ключи API **никогда не вшиваются в образ**. Каждый запускает со своими ключами через файл `.env` на своей машине.

## Быстрый старт (рекомендуется)

Нужен [Docker Desktop](https://www.docker.com/products/docker-desktop/) (или Docker Engine + Compose).

### Windows

```powershell
copy .env.example .env
# отредактируй .env — вставь свои ключи
.\scripts\start.ps1
```

### Linux / macOS

```bash
cp .env.example .env
# отредактируй .env — вставь свои ключи
chmod +x scripts/*.sh
./scripts/start.sh
```

Откроется:

| Сервис | URL |
|--------|-----|
| Proxy / API | http://localhost:8080 |
| Swagger | http://localhost:8080/docs |
| Architecture | http://localhost:8080/architecture |
| pgAdmin | http://localhost:5050 (`admin@example.com` / `admin`) |

Остановка: `.\scripts\stop.ps1` или `./scripts/stop.sh`.

## Куда класть токены

Только в **локальный** `.env` (он в `.gitignore`):

```env
GEMINI_API_KEY=...
GROQ_API_KEY=...
OPENROUTER_API_KEY=...
```

Достаточно **одного** ключа. Остальные можно оставить пустыми.

Compose подставляет `.env` в контейнер `proxy`. Внутри контейнера хост БД всегда `postgres` (это переопределяется в `docker-compose.yml`, даже если в `.env` написано `localhost` для локальной разработки).

## Как передать другу

### Вариант A — папка/репозиторий (проще всего)

1. Отдай клон репо или zip **без** своего `.env`.
2. Друг ставит Docker Desktop.
3. Друг делает `.env` из `.env.example` со **своими** ключами.
4. `.\scripts\start.ps1` / `./scripts/start.sh`.

### Вариант B — готовый Docker-образ (без компиляции Go)

У тебя:

```powershell
.\scripts\save-image.ps1
# получится llm-proxy-image.tar
```

Другу отдаёшь:

- `llm-proxy-image.tar`
- `docker-compose.yml`
- `.env.example`
- `scripts/` (опционально)

У друга:

```bash
docker load -i llm-proxy-image.tar
cp .env.example .env   # свои ключи
docker compose up -d   # proxy уже image: llm-proxy:local
```

Postgres и pgAdmin всё равно скачаются с Docker Hub при первом запуске.

## pgAdmin → Postgres

Add Server:

- Host: `localhost` (с хоста) или `postgres` (если коннект из другого контейнера)
- Port: `5432` (или `DB_PORT` из `.env`)
- User / Password / Database: `llmproxy` / `llmproxy` / `llmproxy`

## Логи

```bash
docker compose logs -f proxy
```

Формат: `req_id=... level=info msg=chat_done provider=... model=...`.

- Свой id: `curl -H "X-Request-ID: my-debug-1" ...`
- Подробности: в `.env` поставь `LOG_LEVEL=debug` и `docker compose up -d --force-recreate proxy`

## Опциональный API-ключ прокси

В `.env` друга:

```env
PROXY_API_KEY=super-secret
```

Тогда вызовы:

```bash
curl -H "Authorization: Bearer super-secret" http://localhost:8080/v1/models
```

`/docs` и `/healthz` остаются без ключа.

## Частые проблемы

| Симптом | Что сделать |
|---------|-------------|
| `at least one provider API key is required` | Заполни ключ в `.env`, `docker compose up -d --force-recreate proxy` |
| Порт 8080 занят | Смени `PROXY_PORT` в `.env` |
| pgAdmin Restarting | `docker compose down -v` и снова start (сотрёт данные БД) |
| Старый бинарь | `docker compose build --no-cache proxy && docker compose up -d` |
