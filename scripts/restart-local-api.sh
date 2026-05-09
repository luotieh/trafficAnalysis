#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

BIN_PATH="${BIN_PATH:-/tmp/traffic-api}"
LOG_PATH="${LOG_PATH:-/tmp/traffic-api.log}"
PID_PATH="${PID_PATH:-/tmp/traffic-api.pid}"
GOCACHE="${GOCACHE:-/tmp/traffic-go-gocache}"
START_DEPS="${START_DEPS:-true}"
export GOCACHE

if [ -f .env ]; then
  set -a
  # shellcheck disable=SC1091
  . ./.env
  set +a
fi

APP_ADDR="${APP_ADDR:-:9010}"
STORE_BACKEND="${STORE_BACKEND:-postgres}"
DATABASE_URL="${DATABASE_URL:-postgres://traffic:traffic@127.0.0.1:5432/traffic_analysis?sslmode=disable}"
AUTO_MIGRATE="${AUTO_MIGRATE:-true}"
MQ_BACKEND="${MQ_BACKEND:-rabbitmq}"
RABBITMQ_URL="${RABBITMQ_URL:-amqp://traffic:traffic@127.0.0.1:5672/}"
RABBITMQ_EXCHANGE="${RABBITMQ_EXCHANGE:-traffic.events}"
RABBITMQ_EVENT_QUEUE="${RABBITMQ_EVENT_QUEUE:-traffic.events.default}"
RABBITMQ_CONSUMER_ENABLED="${RABBITMQ_CONSUMER_ENABLED:-true}"
INTERNAL_API_KEY="${INTERNAL_API_KEY:-change-me-internal-key}"
LLM_TIMEOUT_SECONDS="${LLM_TIMEOUT_SECONDS:-60}"

# .env is also used by docker compose, where service names such as postgres and
# rabbitmq are valid DNS names. A locally started Go process must use localhost.
if [[ "$DATABASE_URL" == *"@postgres:"* ]]; then
  DATABASE_URL="${DATABASE_URL/@postgres:/@127.0.0.1:}"
fi
if [[ "$RABBITMQ_URL" == *"@rabbitmq:"* ]]; then
  RABBITMQ_URL="${RABBITMQ_URL/@rabbitmq:/@127.0.0.1:}"
fi

export APP_ADDR STORE_BACKEND DATABASE_URL AUTO_MIGRATE
export MQ_BACKEND RABBITMQ_URL RABBITMQ_EXCHANGE RABBITMQ_EVENT_QUEUE RABBITMQ_CONSUMER_ENABLED
export INTERNAL_API_KEY LLM_TIMEOUT_SECONDS

port_from_addr() {
  local addr="$1"
  addr="${addr##*:}"
  if [ -z "$addr" ] || [ "$addr" = "$APP_ADDR" ]; then
    addr="9010"
  fi
  printf '%s\n' "$addr"
}

APP_PORT="$(port_from_addr "$APP_ADDR")"

echo "== build local binary =="
go test ./...
go build -o "$BIN_PATH" ./cmd/traffic-api

if [ "$START_DEPS" = "true" ] && command -v docker >/dev/null 2>&1; then
  if docker ps >/dev/null 2>&1; then
    echo "== start local dependencies =="
    if ! docker compose -f deploy/docker-compose.yml up -d --wait postgres rabbitmq; then
      docker compose -f deploy/docker-compose.yml up -d postgres rabbitmq
    fi
  fi
fi

echo "== stop existing API on port $APP_PORT =="
if command -v docker >/dev/null 2>&1; then
  container_id="$(docker ps --filter name=traffic-go --format '{{.ID}}' 2>/dev/null | head -n 1 || true)"
  if [ -n "$container_id" ]; then
    docker stop "$container_id" >/dev/null 2>&1 || true
  fi
fi

pids=""
if command -v lsof >/dev/null 2>&1; then
  pids="$(lsof -tiTCP:"$APP_PORT" -sTCP:LISTEN || true)"
elif command -v ss >/dev/null 2>&1; then
  pids="$(ss -ltnp "sport = :$APP_PORT" | sed -n 's/.*pid=\([0-9][0-9]*\).*/\1/p' | sort -u || true)"
fi

if [ -n "$pids" ]; then
  kill $pids || true
  for _ in 1 2 3 4 5; do
    sleep 1
    alive=""
    for pid in $pids; do
      if kill -0 "$pid" 2>/dev/null; then
        alive="$alive $pid"
      fi
    done
    [ -z "$alive" ] && break
  done
  for pid in $pids; do
    if kill -0 "$pid" 2>/dev/null; then
      kill -KILL "$pid" || true
    fi
  done
fi

echo "== start local API =="
if command -v setsid >/dev/null 2>&1; then
  setsid "$BIN_PATH" </dev/null >"$LOG_PATH" 2>&1 &
else
  nohup "$BIN_PATH" >"$LOG_PATH" 2>&1 &
fi
pid="$!"
printf '%s\n' "$pid" >"$PID_PATH"

health_url="http://127.0.0.1:$APP_PORT/healthz"
for _ in 1 2 3 4 5 6 7 8 9 10; do
  if ! kill -0 "$pid" 2>/dev/null; then
    echo "API failed to start. Last log lines:" >&2
    tail -n 80 "$LOG_PATH" >&2 || true
    exit 1
  fi
  if command -v curl >/dev/null 2>&1; then
    if curl -fsS --max-time 2 "$health_url" >/dev/null 2>&1; then
      break
    fi
  else
    sleep 1
    break
  fi
  sleep 1
done

if command -v curl >/dev/null 2>&1; then
  if ! curl -fsS --max-time 2 "$health_url" >/dev/null 2>&1; then
    echo "API health check failed. Last log lines:" >&2
    tail -n 80 "$LOG_PATH" >&2 || true
    exit 1
  fi
fi

echo "pid=$pid"
echo "log=$LOG_PATH"
echo "health=$health_url"
