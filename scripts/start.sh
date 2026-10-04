#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

if ! docker info >/dev/null 2>&1; then
  echo "Docker не запущен. Установи/открой Docker и повтори."
  exit 1
fi

if [[ ! -f .env ]]; then
  cp .env.example .env
  echo ""
  echo "Создан файл .env из .env.example"
  echo "Открой .env и вставь хотя бы один ключ:"
  echo "  GEMINI_API_KEY / GROQ_API_KEY / OPENROUTER_API_KEY"
  echo "Потом снова запусти: ./scripts/start.sh"
  echo ""
  exit 1
fi

has_key=0
for name in GEMINI_API_KEY GROQ_API_KEY OPENROUTER_API_KEY; do
  if grep -Eq "^[[:space:]]*${name}=[^[:space:]]+" .env; then
    has_key=1
    break
  fi
done
if [[ "$has_key" -ne 1 ]]; then
  echo "В .env нет ключей провайдеров. Заполни хотя бы один и перезапусти."
  exit 1
fi

echo "Собираю и поднимаю llm-proxy (proxy + postgres + pgAdmin)..."
docker compose up -d --build

proxy_port="$(grep -E '^[[:space:]]*PROXY_PORT=' .env | tail -n1 | cut -d= -f2 | tr -d '[:space:]')"
pg_port="$(grep -E '^[[:space:]]*PGADMIN_PORT=' .env | tail -n1 | cut -d= -f2 | tr -d '[:space:]')"
proxy_port="${proxy_port:-8080}"
pg_port="${pg_port:-5050}"

echo ""
echo "Готово."
echo "  Proxy:   http://localhost:${proxy_port}"
echo "  Docs:    http://localhost:${proxy_port}/docs"
echo "  pgAdmin: http://localhost:${pg_port}  (admin@example.com / admin)"
echo "  В pgAdmin Host БД = localhost (порт из DB_PORT), user/pass/db = llmproxy"
echo ""
echo "Логи: docker compose logs -f proxy"
echo "Стоп:  ./scripts/stop.sh"
