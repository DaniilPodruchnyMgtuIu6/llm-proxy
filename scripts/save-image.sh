#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

echo "Собираю llm-proxy:local ..."
docker compose build proxy
out="$(pwd)/llm-proxy-image.tar"
docker save llm-proxy:local -o "$out"
echo "Образ сохранён: $out"
echo "Другу: docker load -i llm-proxy-image.tar"
echo "Потом всё равно нужны docker-compose.yml + .env и: docker compose up -d"
