#!/usr/bin/env bash
set -euo pipefail

COMPOSE_FILE="${COMPOSE_FILE:-deploy/docker-compose.yml}"

echo "[reset-demo] destroying containers and volumes..."
docker compose -f "$COMPOSE_FILE" down -v

echo "[reset-demo] starting postgres and rabbitmq..."
docker compose -f "$COMPOSE_FILE" up -d postgres rabbitmq

echo "[reset-demo] waiting for postgres..."
until docker compose -f "$COMPOSE_FILE" exec -T postgres pg_isready -U traffic -d traffic >/dev/null 2>&1; do
  sleep 2
done

echo "[reset-demo] waiting for rabbitmq..."
until docker compose -f "$COMPOSE_FILE" exec -T rabbitmq rabbitmq-diagnostics -q ping >/dev/null 2>&1; do
  sleep 2
done

echo "[reset-demo] building traffic-go..."
docker compose -f "$COMPOSE_FILE" build traffic-go

echo "[reset-demo] initializing database and demo data..."
docker compose -f "$COMPOSE_FILE" run --rm --entrypoint /traffic-admin traffic-go -init-with-demo

echo "[reset-demo] starting api service..."
docker compose -f "$COMPOSE_FILE" up -d traffic-go

echo "[reset-demo] done."
docker compose -f "$COMPOSE_FILE" ps
