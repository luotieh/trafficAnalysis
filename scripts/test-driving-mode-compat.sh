#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:9010}"
USERNAME="${ADMIN_USERNAME:-admin}"
PASSWORD="${ADMIN_PASSWORD:-admin123}"
EVENT_ID="driving-test-$(date +%s)"

echo "== Driving mode compatibility test =="
echo "BASE_URL=$BASE_URL EVENT_ID=$EVENT_ID"

TOKEN=$(curl -s -X POST "$BASE_URL/api/auth/login" \
  -H "Content-Type: application/json" \
  -d "{\"username\":\"$USERNAME\",\"password\":\"$PASSWORD\"}" \
  | sed -n 's/.*"access_token":"\([^"]*\)".*/\1/p')

if [ -z "$TOKEN" ]; then
  echo "FAIL: login failed, token empty"
  exit 1
fi

echo "== enable driving mode =="
curl -s -X PUT "$BASE_URL/api/state/driving-mode" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"enabled":true}' | tee /tmp/driving-mode-enable.json
echo

echo "== check driving mode =="
curl -s "$BASE_URL/api/state/driving-mode" \
  -H "Authorization: Bearer $TOKEN" | tee /tmp/driving-mode-state.json
echo

python3 - <<'PY'
import json
from pathlib import Path
state=json.loads(Path('/tmp/driving-mode-state.json').read_text())
if not state.get('data',{}).get('enabled'):
    raise SystemExit('FAIL: driving mode was not persisted as enabled=true')
print('PASS: driving mode persisted enabled=true')
PY

echo "== create event =="
curl -s -X POST "$BASE_URL/api/event/create" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d "{
    \"event_id\":\"$EVENT_ID\",
    \"event_name\":\"自动驾驶检测事件\",
    \"title\":\"自动驾驶检测事件\",
    \"message\":\"用于检测开启自动驾驶后是否自动生成任务、执行和总结。\",
    \"source\":\"driving-mode-test\",
    \"severity\":\"high\",
    \"category\":\"Network Threat\",
    \"context\":\"{\\\"test\\\":true}\",
    \"observables\":[
      {\"type\":\"ip\",\"value\":\"66.240.205.34\",\"role\":\"source\"},
      {\"type\":\"ip\",\"value\":\"172.16.20.20\",\"role\":\"destination\"}
    ]
  }" | tee /tmp/driving-event-create.json
echo

echo "== wait for automation =="
sleep 3

echo "== check tasks =="
curl -s "$BASE_URL/api/event/$EVENT_ID/tasks" \
  -H "Authorization: Bearer $TOKEN" | tee /tmp/driving-tasks.json
echo

echo "== check executions =="
curl -s "$BASE_URL/api/event/$EVENT_ID/executions" \
  -H "Authorization: Bearer $TOKEN" | tee /tmp/driving-executions.json
echo

echo "== check summaries =="
curl -s "$BASE_URL/api/event/$EVENT_ID/summaries" \
  -H "Authorization: Bearer $TOKEN" | tee /tmp/driving-summaries.json
echo

echo "== check messages =="
curl -s "$BASE_URL/api/event/$EVENT_ID/messages" \
  -H "Authorization: Bearer $TOKEN" | tee /tmp/driving-messages.json
echo

python3 - <<'PY'
import json
from pathlib import Path

def load(path):
    try:
        return json.loads(Path(path).read_text())
    except Exception:
        return {}

tasks = load('/tmp/driving-tasks.json').get('data', [])
executions = load('/tmp/driving-executions.json').get('data', [])
summaries = load('/tmp/driving-summaries.json').get('data', [])
messages = load('/tmp/driving-messages.json').get('data', [])

print('== summary ==')
print('tasks:', len(tasks) if isinstance(tasks, list) else 'unknown')
print('executions:', len(executions) if isinstance(executions, list) else 'unknown')
print('summaries:', len(summaries) if isinstance(summaries, list) else 'unknown')
print('messages:', len(messages) if isinstance(messages, list) else 'unknown')

if not isinstance(tasks, list) or len(tasks) < 1:
    raise SystemExit('FAIL: no autopilot task was generated')
if not isinstance(executions, list) or len(executions) < 1:
    raise SystemExit('FAIL: no autopilot execution was generated')
if not isinstance(summaries, list) or len(summaries) < 1:
    raise SystemExit('FAIL: no autopilot summary was generated')
if not isinstance(messages, list) or len(messages) < 3:
    raise SystemExit('FAIL: no clear autopilot message was generated')

print('PASS: driving mode triggers task/execution/summary/message automation artifacts.')
PY

echo "== disable driving mode =="
curl -s -X PUT "$BASE_URL/api/state/driving-mode" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"enabled":false}'
echo
