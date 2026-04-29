#!/usr/bin/env bash
set -euo pipefail

BASE="${BASE:-http://localhost:9010}"

echo "== health =="
curl -s "$BASE/healthz"; echo

echo "== login =="
TOKEN=$(curl -s -X POST "$BASE/api/auth/login"   -H 'Content-Type: application/json'   -d '{"username":"admin","password":"admin"}' | sed -n 's/.*"access_token":"\([^"]*\)".*/\1/p')
echo "token length: ${#TOKEN}"

echo "== create event =="
curl -s -X POST "$BASE/api/event/create"   -H "Content-Type: application/json"   -H "Authorization: Bearer $TOKEN"   -d '{"event_name":"smoke-test","message":"数据库持久化冒烟测试","severity":"high","source":"manual"}'; echo

echo "== list events =="
curl -s "$BASE/api/event/list"; echo
