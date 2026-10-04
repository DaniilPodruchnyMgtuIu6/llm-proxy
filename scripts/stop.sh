#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
docker compose down
echo "Остановлено. Данные Postgres сохранены в volume."
echo "Полный сброс БД: docker compose down -v"
