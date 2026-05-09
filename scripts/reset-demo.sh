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

wait_cmd() {
  local service="$1"
  local label="$2"
  local cmd="$3"
  local timeout="${4:-180}"
  local start
  start=$(date +%s)

  echo "[reset-demo] waiting for $label..."
  until eval "$cmd" >/dev/null 2>&1; do
    if [ $(( $(date +%s) - start )) -gt "$timeout" ]; then
      echo "[reset-demo] timeout waiting for $label" >&2
      docker compose -f "$COMPOSE_FILE" ps || true
      docker compose -f "$COMPOSE_FILE" logs "$service" --tail=120 || true
      exit 1
    fi
    sleep 2
  done
}

echo "[reset-demo] destroying containers and volumes..."
docker compose -f "$COMPOSE_FILE" down -v --remove-orphans

echo "[reset-demo] starting postgres and rabbitmq..."
docker compose -f "$COMPOSE_FILE" up -d --pull never postgres rabbitmq

wait_cmd postgres "postgres" "docker compose -f '$COMPOSE_FILE' exec -T postgres pg_isready -U traffic -d traffic_analysis" 120
wait_cmd rabbitmq "rabbitmq" "docker compose -f '$COMPOSE_FILE' exec -T rabbitmq rabbitmq-diagnostics -q ping" 180

echo "[reset-demo] building local traffic-admin..."
go build -o "$ADMIN_BIN" ./cmd/traffic-admin

echo "[reset-demo] initializing PostgreSQL, RabbitMQ and ly_server PostgreSQL-compatible schema..."
"$ADMIN_BIN" -init-with-demo -init-lyserver-compat

echo "[reset-demo] restarting local api service..."
START_DEPS=false scripts/restart-local-api.sh

echo "[reset-demo] done."
docker compose -f "$COMPOSE_FILE" ps
