# Plan: reliability

- **ID:** `2026-10-04T203100-reliability`
- **Created:** 2026-10-04T20:31:00+03:00
- **Status:** in_progress
- **Branch:** `feat/reliability`
- **Supersedes archive:** `archive/2026-10-04-fallback-route-limits`

## Goal

Стабильность free-proxy без streaming:

1. Upstream timeout + лимит fallback-попыток.
2. Не слать в модели с `remaining_rpm/tpm/rpd == 0` (live headers / local).
3. Писать usage **на каждую** attempt (включая проигравшие 429).
4. Опциональный `PROXY_API_KEY` на `/v1/*`.
5. In-memory cache `GET /v1/models` (TTL).
6. Простой circuit breaker по `provider_health`.

## Out of scope

- Streaming SSE
- Prometheus
- Multi-user auth / UI ключей
