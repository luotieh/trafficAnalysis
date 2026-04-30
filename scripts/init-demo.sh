#!/usr/bin/env bash
set -euo pipefail

COMPOSE_FILE="${COMPOSE_FILE:-deploy/docker-compose.yml}"

echo "[init-demo] starting postgres, mysql and rabbitmq..."
docker compose -f "$COMPOSE_FILE" up -d postgres mysql rabbitmq

echo "[init-demo] initializing demo data..."
docker compose -f "$COMPOSE_FILE" run --rm --entrypoint /traffic-admin traffic-go -init-with-demo -init-lyserver-db

echo "[init-demo] starting traffic-go..."
docker compose -f "$COMPOSE_FILE" up -d traffic-go

echo "[init-demo] done. API: http://localhost:9010/healthz RabbitMQ UI: http://localhost:15672 MySQL: localhost:3306/server"
