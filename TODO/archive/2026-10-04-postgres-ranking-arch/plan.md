# Plan: postgres-ranking-arch

- **ID:** `2026-10-04T194300-postgres-ranking-arch`
- **Created:** 2026-10-04T19:43:00+03:00
- **Status:** archived
- **Archived:** 2026-10-04T19:58:00+03:00

## Goal

1. Перейти с SQLite на **PostgreSQL** (Docker Compose + pgAdmin).
2. Сортировать модели **от сильных к слабым**.
3. Задокументировать архитектуру и алгоритмы со **схемами Mermaid**.
4. Обновить README, `docs/*`, OpenAPI/Swagger.
5. Раздельные `DB_*` env (не одна `DATABASE_URL`).
6. Viewer `/architecture` для рендера Mermaid.

## Done

- Postgres + docker-compose + pgAdmin
- Ranking `quality_score` / `rank`
- `docs/architecture.md` + `/architecture`
- AGENTS.md / TODO workflow
