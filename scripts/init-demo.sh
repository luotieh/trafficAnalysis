#!/usr/bin/env bash
set -euo pipefail

COMPOSE_FILE="${COMPOSE_FILE:-deploy/docker-compose.yml}"
ADMIN_BIN="${ADMIN_BIN:-/tmp/traffic-admin}"
GOFLAGS="${GOFLAGS:--mod=vendor}"
GOCACHE="${GOCACHE:-/tmp/traffic-go-gocache}"
export GOFLAGS GOCACHE

if [ -f .env ]; then
  set -a
  # shellcheck disable=SC1091
  . ./.env
  set +a
fi

DATABASE_URL="${DATABASE_URL:-postgres://traffic:traffic@127.0.0.1:5432/traffic_analysis?sslmode=disable}"
RABBITMQ_URL="${RABBITMQ_URL:-amqp://traffic:traffic@127.0.0.1:5672/}"
if [[ "$DATABASE_URL" == *"@postgres:"* ]]; then
  DATABASE_URL="${DATABASE_URL/@postgres:/@127.0.0.1:}"
fi
if [[ "$RABBITMQ_URL" == *"@rabbitmq:"* ]]; then
  RABBITMQ_URL="${RABBITMQ_URL/@rabbitmq:/@127.0.0.1:}"
fi
export DATABASE_URL RABBITMQ_URL

echo "[init-demo] starting postgres and rabbitmq..."
docker compose -f "$COMPOSE_FILE" up -d --pull never postgres rabbitmq

echo "[init-demo] building local traffic-admin..."
go build -o "$ADMIN_BIN" ./cmd/traffic-admin

echo "[init-demo] initializing demo data..."
"$ADMIN_BIN" -init-with-demo -init-lyserver-compat

echo "[init-demo] restarting local traffic-go API..."
START_DEPS=false scripts/restart-local-api.sh

echo "[init-demo] done. API: http://localhost:9010/healthz RabbitMQ UI: http://localhost:15672"
