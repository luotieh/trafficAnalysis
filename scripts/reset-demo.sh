#!/usr/bin/env bash
set -euo pipefail

COMPOSE_FILE="${COMPOSE_FILE:-deploy/docker-compose.yml}"

echo "[reset-demo] destroying containers and volumes..."
docker compose -f "$COMPOSE_FILE" down -v

echo "[reset-demo] rebuilding and initializing with demo data..."
docker compose -f "$COMPOSE_FILE" up -d --build postgres rabbitmq
docker compose -f "$COMPOSE_FILE" run --rm --no-deps --entrypoint /traffic-admin \
  traffic-go -init-with-demo
docker compose -f "$COMPOSE_FILE" up -d traffic-go

echo "[reset-demo] done."
