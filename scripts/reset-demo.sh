#!/usr/bin/env bash
set -euo pipefail

COMPOSE_FILE="${COMPOSE_FILE:-deploy/docker-compose.yml}"

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

echo "[reset-demo] starting postgres, mysql and rabbitmq..."
docker compose -f "$COMPOSE_FILE" up -d postgres mysql rabbitmq

wait_cmd postgres "postgres" "docker compose -f '$COMPOSE_FILE' exec -T postgres pg_isready -U traffic -d traffic_analysis" 120
wait_cmd mysql "mysql" "docker compose -f '$COMPOSE_FILE' exec -T mysql mysqladmin ping -h 127.0.0.1 -utraffic -ptraffic --silent" 180
wait_cmd rabbitmq "rabbitmq" "docker compose -f '$COMPOSE_FILE' exec -T rabbitmq rabbitmq-diagnostics -q ping" 180

echo "[reset-demo] building traffic-go..."
docker compose -f "$COMPOSE_FILE" build traffic-go

echo "[reset-demo] initializing PostgreSQL, RabbitMQ and ly_server MySQL data..."
docker compose -f "$COMPOSE_FILE" run --rm --entrypoint /traffic-admin traffic-go -init-with-demo -init-lyserver-db

echo "[reset-demo] starting api service..."
docker compose -f "$COMPOSE_FILE" up -d traffic-go

echo "[reset-demo] done."
docker compose -f "$COMPOSE_FILE" ps
