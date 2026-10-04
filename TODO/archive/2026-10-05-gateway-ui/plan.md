# Plan: gateway-ui

- **ID:** `2026-10-04T231000-gateway-ui`
- **Created:** 2026-10-04T23:10:00+03:00
- **Status:** archived
- **Branch:** `feat/gateway-ui`
- **Supersedes archive:** `archive/2026-10-04-reliability`

## Decisions

1. Кнопка Apply = сохранить + hot-reload провайдеров (без `docker compose up`).
2. UI = отдельный сервис `web` в compose (nginx + SPA).
3. Ключи → volume (`RUNTIME_DATA_DIR`) + reload в процессе proxy.
4. Страница статистики: модели, remaining, summary.

## Goal

Лаконичный UI: wizard ключей → дашборд квот → настройки gateway (model/params) → stats → test chat + подсказки по API.

## Out of scope

- Streaming в test chat
- Управление Docker daemon / docker.sock
- Multi-user
