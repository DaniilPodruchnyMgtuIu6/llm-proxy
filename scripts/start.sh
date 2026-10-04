#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

if ! docker info >/dev/null 2>&1; then
  echo "Docker не запущен. Установи/открой Docker и повтори."
  exit 1
fi

if [[ ! -f .env ]]; then
  cp .env.example .env
  echo "Создан .env из .env.example (ключи можно ввести в UI)."
fi

echo "Собираю и поднимаю llm-proxy (web UI + proxy + postgres + pgAdmin)..."
docker compose up -d --build

proxy_port="$(grep -E '^[[:space:]]*PROXY_PORT=' .env | tail -n1 | cut -d= -f2 | tr -d '[:space:]' || true)"
ui_port="$(grep -E '^[[:space:]]*UI_PORT=' .env | tail -n1 | cut -d= -f2 | tr -d '[:space:]' || true)"
pg_port="$(grep -E '^[[:space:]]*PGADMIN_PORT=' .env | tail -n1 | cut -d= -f2 | tr -d '[:space:]' || true)"
proxy_port="${proxy_port:-8080}"
ui_port="${ui_port:-3000}"
pg_port="${pg_port:-5050}"

echo ""
echo "Готово."
echo "  UI:      http://localhost:${ui_port}"
echo "  Proxy:   http://localhost:${proxy_port}"
echo "  Docs:    http://localhost:${ui_port}/docs"
echo "  pgAdmin: http://localhost:${pg_port}  (admin@example.com / admin)"
echo ""
echo "Ключи провайдеров: открой UI → Setup/Ключи → Применить (без docker compose up)."
echo "Логи: docker compose logs -f proxy web"
echo "Стоп:  ./scripts/stop.sh"
