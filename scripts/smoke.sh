#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:9010}"

echo "== health =="
curl -fsS "$BASE_URL/healthz" | tee /tmp/traffic-health.json
echo

echo "== login =="
TOKEN=$(curl -fsS -X POST "$BASE_URL/api/auth/login" \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"admin"}' | sed -n 's/.*"access_token":"\([^"]*\)".*/\1/p')
if [ -z "$TOKEN" ]; then
  echo "login failed: token empty" >&2
  exit 1
fi

echo "== create event =="
EVENT_ID=$(curl -fsS -X POST "$BASE_URL/api/event/create" \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"event_name":"MQ冒烟测试事件","message":"检测到异常流量","severity":"high","source":"smoke"}' \
  | sed -n 's/.*"event_id":"\([^"]*\)".*/\1/p')
echo "event_id=$EVENT_ID"

# RabbitMQ worker 异步消费需要一点时间。
sleep 2

echo "== list events =="
curl -fsS "$BASE_URL/api/event/list"
echo

if [ -n "$EVENT_ID" ]; then
  echo "== event messages =="
  curl -fsS "$BASE_URL/api/event/$EVENT_ID/messages"
  echo
fi
