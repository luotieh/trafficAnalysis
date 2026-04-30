#!/usr/bin/env bash
set -euo pipefail

# 原三端接口兼容性检查脚本
#
# 目标：
#   1. 检查 Go 重构版是否覆盖原 adapter / deepflowsoc / ly_server 的主要接口路径。
#   2. 区分“本地可用接口”“需要上游依赖的代理接口”“占位接口”“失败接口”。
#   3. 生成 Markdown 报告，便于后续逐项补齐。
#
# 用法：
#   chmod +x scripts/test-original-api-compat.sh
#   ./scripts/test-original-api-compat.sh
#
# 可配置环境变量：
#   BASE_URL=http://localhost:9010
#   INTERNAL_API_KEY=change-me-internal-key
#   ADMIN_USERNAME=admin
#   ADMIN_PASSWORD=admin123
#   ADMIN_PASSWORD_FALLBACK=admin
#   REPORT_DIR=reports
#   CURL_MAX_TIME=20
#   CURL_CONNECT_TIMEOUT=5
#
# 判定模式：
#   must_2xx          : 必须 2xx，否则 FAIL
#   auth_optional     : 2xx/401/403 可接受；401/403 记 WARN
#   upstream_optional : 2xx 记 PASS；502/503/504 记 WARN，表示上游未配置或不可达
#   placeholder_ok    : 2xx/202 记 PASS/WARN，用于当前保留兼容但业务未完全实现的接口
#   any_http          : 只要能返回 HTTP 状态就 PASS，用于探测路由是否挂载

BASE_URL="${BASE_URL:-http://localhost:9010}"
INTERNAL_API_KEY="${INTERNAL_API_KEY:-change-me-internal-key}"
ADMIN_USERNAME="${ADMIN_USERNAME:-admin}"
ADMIN_PASSWORD="${ADMIN_PASSWORD:-admin123}"
ADMIN_PASSWORD_FALLBACK="${ADMIN_PASSWORD_FALLBACK:-admin}"
REPORT_DIR="${REPORT_DIR:-reports}"
CURL_MAX_TIME="${CURL_MAX_TIME:-20}"
CURL_CONNECT_TIMEOUT="${CURL_CONNECT_TIMEOUT:-5}"
MYSQL_READY_CHECK="${MYSQL_READY_CHECK:-true}"
MYSQL_SERVICE="${MYSQL_SERVICE:-mysql}"
MYSQL_USER="${MYSQL_USER:-traffic}"
MYSQL_PASSWORD="${MYSQL_PASSWORD:-traffic}"
MYSQL_DATABASE="${MYSQL_DATABASE:-server}"

mkdir -p "$REPORT_DIR"

RUN_ID="$(date +%Y%m%d-%H%M%S)"
REPORT_FILE="$REPORT_DIR/original-api-compat-$RUN_ID.md"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

PASS=0
WARN=0
FAIL=0
TOTAL=0

TOKEN=""
EVENT_ID="${EVENT_ID:-demo-event-001}"
TEST_USER_ID=""
TEST_USERNAME="compat_user_$(date +%s)"

supports_color() {
  [[ -t 1 ]] && [[ "${NO_COLOR:-}" == "" ]]
}

if supports_color; then
  C_PASS=$'\033[32m'
  C_WARN=$'\033[33m'
  C_FAIL=$'\033[31m'
  C_INFO=$'\033[36m'
  C_RESET=$'\033[0m'
else
  C_PASS=""
  C_WARN=""
  C_FAIL=""
  C_INFO=""
  C_RESET=""
fi

urlencode() {
  python3 - "$1" <<'PY'
import sys, urllib.parse
print(urllib.parse.quote(sys.argv[1], safe=''))
PY
}

json_get() {
  local expr="$1"
  local file="$2"
  python3 - "$expr" "$file" <<'PY'
import json, sys
expr, path = sys.argv[1], sys.argv[2]
try:
    with open(path, "r", encoding="utf-8") as f:
        data = json.load(f)
except Exception:
    print("")
    raise SystemExit(0)

cur = data
for part in expr.split("."):
    if part == "":
        continue
    if isinstance(cur, dict):
        cur = cur.get(part, "")
    elif isinstance(cur, list):
        try:
            cur = cur[int(part)]
        except Exception:
            cur = ""
    else:
        cur = ""
if cur is None:
    cur = ""
print(cur if isinstance(cur, str) else json.dumps(cur, ensure_ascii=False))
PY
}

is_json_body() {
  local file="$1"
  python3 - "$file" <<'PY'
import json, sys
try:
    raw = open(sys.argv[1], "rb").read().strip()
    if not raw:
        raise ValueError("empty")
    json.loads(raw.decode("utf-8"))
    print("yes")
except Exception:
    print("no")
PY
}

write_report_header() {
  cat > "$REPORT_FILE" <<EOF
# 原三端接口兼容性检查报告

- 时间：$(date -Iseconds)
- BASE_URL：$BASE_URL
- 事件样本：$EVENT_ID
- 说明：PASS 表示接口当前可用；WARN 表示路由存在但依赖上游、鉴权或业务仍需补齐；FAIL 表示不符合当前替代目标。

| 结果 | 分组 | 接口 | HTTP | 说明 |
|---|---|---|---:|---|
EOF
}

append_report_row() {
  local result="$1"
  local group="$2"
  local name="$3"
  local status="$4"
  local note="$5"
  # 避免 Markdown 表格被响应内容破坏
  note="${note//$'\n'/ }"
  note="${note//|/\\|}"
  echo "| $result | $group | \`$name\` | $status | $note |" >> "$REPORT_FILE"
}


record_infra_result() {
  local result="$1"
  local group="$2"
  local name="$3"
  local status="$4"
  local note="$5"

  TOTAL=$((TOTAL + 1))
  case "$result" in
    PASS) PASS=$((PASS + 1)) ;;
    WARN) WARN=$((WARN + 1)) ;;
    FAIL) FAIL=$((FAIL + 1)) ;;
  esac
  print_result "$result" "$group" "$name" "$status" "$note"
  append_report_row "$result" "$group" "$name" "$status" "$note"
}

check_mysql_ready() {
  if [[ "$MYSQL_READY_CHECK" != "true" ]]; then
    record_infra_result "WARN" "infra/mysql" "mysql ready check" "-" "MYSQL_READY_CHECK=false，跳过 ly_server MySQL 检测"
    return
  fi

  if ! command -v docker >/dev/null 2>&1; then
    record_infra_result "WARN" "infra/mysql" "mysql ready check" "-" "未找到 docker 命令，跳过 ly_server MySQL 检测"
    return
  fi

  if docker compose -f deploy/docker-compose.yml exec -T "$MYSQL_SERVICE" \
      mysqladmin ping -h 127.0.0.1 -u"$MYSQL_USER" -p"$MYSQL_PASSWORD" --silent >/dev/null 2>&1; then
    record_infra_result "PASS" "infra/mysql" "mysqladmin ping $MYSQL_SERVICE/$MYSQL_DATABASE" "ready" "ly_server MySQL ready"
  else
    record_infra_result "WARN" "infra/mysql" "mysqladmin ping $MYSQL_SERVICE/$MYSQL_DATABASE" "not-ready" "ly_server MySQL 未就绪，/d/* 原生替代可能无法验证"
  fi
}

print_result() {
  local result="$1"
  local group="$2"
  local name="$3"
  local status="$4"
  local note="$5"

  case "$result" in
    PASS) echo "${C_PASS}[PASS]${C_RESET} [$group] $name -> $status $note" ;;
    WARN) echo "${C_WARN}[WARN]${C_RESET} [$group] $name -> $status $note" ;;
    FAIL) echo "${C_FAIL}[FAIL]${C_RESET} [$group] $name -> $status $note" ;;
    *) echo "[$result] [$group] $name -> $status $note" ;;
  esac
}

classify() {
  local mode="$1"
  local status="$2"
  local json_ok="$3"

  case "$mode" in
    must_2xx)
      if [[ "$status" =~ ^2 ]]; then echo "PASS"; else echo "FAIL"; fi
      ;;
    auth_optional)
      if [[ "$status" =~ ^2 ]]; then echo "PASS"
      elif [[ "$status" == "401" || "$status" == "403" ]]; then echo "WARN"
      else echo "FAIL"; fi
      ;;
    upstream_optional)
      if [[ "$status" =~ ^2 ]]; then echo "PASS"
      elif [[ "$status" == "502" || "$status" == "503" || "$status" == "504" ]]; then echo "WARN"
      else echo "FAIL"; fi
      ;;
    placeholder_ok)
      if [[ "$status" =~ ^2 ]]; then
        if [[ "$status" == "202" ]]; then echo "WARN"; else echo "PASS"; fi
      else
        echo "FAIL"
      fi
      ;;
    any_http)
      if [[ "$status" =~ ^[0-9][0-9][0-9]$ ]]; then echo "PASS"; else echo "FAIL"; fi
      ;;
    *)
      echo "FAIL"
      ;;
  esac
}

request() {
  local group="$1"
  local name="$2"
  local method="$3"
  local path="$4"
  local mode="$5"
  local payload="${6:-}"
  local auth="${7:-none}"
  local extra_header="${8:-}"

  TOTAL=$((TOTAL + 1))

  local body_file="$TMP_DIR/body_$TOTAL.json"
  local header_file="$TMP_DIR/header_$TOTAL.txt"
  local url="$BASE_URL$path"

  local args=(
    -sS
    --connect-timeout "$CURL_CONNECT_TIMEOUT"
    --max-time "$CURL_MAX_TIME"
    -o "$body_file"
    -D "$header_file"
    -w "%{http_code}"
    -X "$method"
    "$url"
  )

  args+=(-H "Accept: application/json")

  if [[ -n "$payload" ]]; then
    args+=(-H "Content-Type: application/json" -d "$payload")
  fi

  if [[ "$auth" == "bearer" && -n "$TOKEN" ]]; then
    args+=(-H "Authorization: Bearer $TOKEN")
  fi

  if [[ "$auth" == "internal" ]]; then
    args+=(-H "X-API-Key: $INTERNAL_API_KEY")
  fi

  if [[ -n "$extra_header" ]]; then
    args+=(-H "$extra_header")
  fi

  local status
  status="$(curl "${args[@]}" 2>"$TMP_DIR/curl_err_$TOTAL.txt" || true)"

  if [[ ! "$status" =~ ^[0-9][0-9][0-9]$ ]]; then
    status="000"
  fi

  local json_ok="no"
  if [[ -s "$body_file" ]]; then
    json_ok="$(is_json_body "$body_file")"
  fi

  local result
  result="$(classify "$mode" "$status" "$json_ok")"

  local note=""
  case "$result" in
    PASS)
      note="ok"
      PASS=$((PASS + 1))
      ;;
    WARN)
      WARN=$((WARN + 1))
      if [[ "$status" == "401" || "$status" == "403" ]]; then
        note="需要鉴权或 token 未生效"
      elif [[ "$status" == "502" || "$status" == "503" || "$status" == "504" ]]; then
        note="路由存在，但依赖上游服务或代理未配置"
      elif [[ "$status" == "202" ]]; then
        note="兼容入口存在，但当前可能是占位/异步接受"
      else
        note="可接受但需复核"
      fi
      ;;
    FAIL)
      FAIL=$((FAIL + 1))
      local curl_err
      curl_err="$(cat "$TMP_DIR/curl_err_$TOTAL.txt" 2>/dev/null || true)"
      local body_preview
      body_preview="$(head -c 240 "$body_file" 2>/dev/null || true)"
      note="不符合预期"
      if [[ -n "$curl_err" ]]; then
        note="$note; curl=$curl_err"
      elif [[ -n "$body_preview" ]]; then
        note="$note; body=$body_preview"
      fi
      ;;
  esac

  if [[ "$json_ok" != "yes" && "$status" != "204" && "$status" != "000" ]]; then
    if [[ "$result" == "PASS" && "$mode" != "any_http" ]]; then
      result="WARN"
      PASS=$((PASS - 1))
      WARN=$((WARN + 1))
      note="返回非 JSON，需要确认是否兼容原调用方"
    fi
  fi

  print_result "$result" "$group" "$name" "$status" "$note"
  append_report_row "$result" "$group" "$method $path" "$status" "$note"

  LAST_STATUS="$status"
  LAST_BODY="$body_file"
}

login() {
  echo "${C_INFO}== 登录获取 token ==${C_RESET}"
  request "deepflowsoc/auth" "POST /api/auth/login admin primary" "POST" "/api/auth/login" "must_2xx" \
    "{\"username\":\"$ADMIN_USERNAME\",\"password\":\"$ADMIN_PASSWORD\"}" "none"

  TOKEN="$(json_get "access_token" "$LAST_BODY")"
  if [[ -z "$TOKEN" ]]; then
    TOKEN="$(json_get "data.access_token" "$LAST_BODY")"
  fi

  if [[ -z "$TOKEN" && "$ADMIN_PASSWORD_FALLBACK" != "$ADMIN_PASSWORD" ]]; then
    request "deepflowsoc/auth" "POST /api/auth/login admin fallback" "POST" "/api/auth/login" "must_2xx" \
      "{\"username\":\"$ADMIN_USERNAME\",\"password\":\"$ADMIN_PASSWORD_FALLBACK\"}" "none"
    TOKEN="$(json_get "access_token" "$LAST_BODY")"
    if [[ -z "$TOKEN" ]]; then
      TOKEN="$(json_get "data.access_token" "$LAST_BODY")"
    fi
  fi

  if [[ -n "$TOKEN" ]]; then
    echo "${C_PASS}token acquired${C_RESET}"
  else
    echo "${C_WARN}token empty，后续 bearer 接口可能出现 WARN/FAIL${C_RESET}"
  fi
}

write_report_header

echo "${C_INFO}== 原三端接口兼容性检查开始 ==${C_RESET}"
echo "BASE_URL=$BASE_URL"

# 基础健康
request "base" "GET /healthz" "GET" "/healthz" "must_2xx"
request "base" "GET /health" "GET" "/health" "must_2xx"
request "base" "GET /api/version" "GET" "/api/version" "must_2xx"
check_mysql_ready

login

# Auth
request "deepflowsoc/auth" "GET /api/auth/check-auth" "GET" "/api/auth/check-auth" "must_2xx" "" "bearer"
request "deepflowsoc/auth" "GET /api/auth/me" "GET" "/api/auth/me" "auth_optional" "" "bearer"
request "deepflowsoc/auth" "POST /api/auth/logout" "POST" "/api/auth/logout" "must_2xx" "{}" "bearer"

# User 兼容
request "deepflowsoc/user" "POST /api/auth/create-user" "POST" "/api/auth/create-user" "placeholder_ok" \
  "{\"username\":\"$TEST_USERNAME\",\"password\":\"Compat123!\",\"nickname\":\"兼容测试用户\",\"email\":\"$TEST_USERNAME@example.local\",\"role\":\"user\"}" "bearer"
TEST_USER_ID="$(json_get "data.user_id" "$LAST_BODY")"
if [[ -z "$TEST_USER_ID" ]]; then
  TEST_USER_ID="$TEST_USERNAME"
fi

request "deepflowsoc/user" "GET /api/user/list" "GET" "/api/user/list" "must_2xx" "" "bearer"
request "deepflowsoc/user" "GET /api/user/{user_id}" "GET" "/api/user/$TEST_USER_ID" "placeholder_ok" "" "bearer"
request "deepflowsoc/user" "PUT /api/user/{user_id}" "PUT" "/api/user/$TEST_USER_ID" "placeholder_ok" \
  "{\"nickname\":\"兼容测试用户-更新\"}" "bearer"
request "deepflowsoc/user" "PUT /api/user/{user_id}/password" "PUT" "/api/user/$TEST_USER_ID/password" "placeholder_ok" \
  "{\"password\":\"Compat456!\"}" "bearer"

# Event 兼容
NEW_EVENT_ID="compat-event-$(date +%s)"
request "deepflowsoc/event" "POST /api/event/create" "POST" "/api/event/create" "must_2xx" \
  "{\"event_id\":\"$NEW_EVENT_ID\",\"event_name\":\"兼容测试事件\",\"title\":\"兼容测试事件\",\"message\":\"接口兼容性脚本创建的事件\",\"severity\":\"medium\",\"source\":\"compat-test\",\"observables\":[{\"type\":\"ip\",\"value\":\"192.0.2.10\",\"role\":\"source\"}]}" "bearer"
EVENT_ID="$NEW_EVENT_ID"

request "deepflowsoc/event" "GET /api/event/list" "GET" "/api/event/list" "must_2xx" "" "bearer"
request "deepflowsoc/event" "GET /api/event/{event_id}" "GET" "/api/event/$EVENT_ID" "must_2xx" "" "bearer"
request "deepflowsoc/event" "GET /api/event/{event_id}/messages" "GET" "/api/event/$EVENT_ID/messages" "must_2xx" "" "bearer"
request "deepflowsoc/event" "POST /api/event/send_message/{event_id}" "POST" "/api/event/send_message/$EVENT_ID" "must_2xx" \
  "{\"message\":\"兼容测试消息\",\"message_from\":\"engineer\",\"message_type\":\"text\"}" "bearer"
request "deepflowsoc/event" "GET /api/event/{event_id}/tasks" "GET" "/api/event/$EVENT_ID/tasks" "must_2xx" "" "bearer"
request "deepflowsoc/event" "GET /api/event/{event_id}/stats" "GET" "/api/event/$EVENT_ID/stats" "must_2xx" "" "bearer"
request "deepflowsoc/event" "GET /api/event/{event_id}/summaries" "GET" "/api/event/$EVENT_ID/summaries" "must_2xx" "" "bearer"
request "deepflowsoc/event" "GET /api/event/{event_id}/executions" "GET" "/api/event/$EVENT_ID/executions" "must_2xx" "" "bearer"
request "deepflowsoc/event" "POST /api/event/{event_id}/execution/{execution_id}/complete" "POST" "/api/event/$EVENT_ID/execution/demo-execution-001/complete" "placeholder_ok" \
  "{\"result\":\"compat test complete\",\"status\":\"completed\"}" "bearer"
request "deepflowsoc/event" "GET /api/event/{event_id}/hierarchy" "GET" "/api/event/$EVENT_ID/hierarchy" "must_2xx" "" "bearer"

# Prompt / State / Report
request "deepflowsoc/prompt" "GET /api/prompt/list" "GET" "/api/prompt/list" "must_2xx" "" "bearer"
request "deepflowsoc/prompt" "GET /api/prompt/{role}" "GET" "/api/prompt/role_soc_captain" "must_2xx" "" "bearer"
request "deepflowsoc/prompt" "PUT /api/prompt/{role}" "PUT" "/api/prompt/compat_test_role" "placeholder_ok" \
  "{\"content\":\"compat test prompt\"}" "bearer"
request "deepflowsoc/prompt" "GET /api/prompt/background/{name}" "GET" "/api/prompt/background/security" "must_2xx" "" "bearer"
request "deepflowsoc/prompt" "PUT /api/prompt/background/{name}" "PUT" "/api/prompt/background/compat_test" "placeholder_ok" \
  "{\"content\":\"compat test background\"}" "bearer"

request "deepflowsoc/state" "GET /api/state/driving-mode" "GET" "/api/state/driving-mode" "must_2xx" "" "bearer"
request "deepflowsoc/state" "PUT /api/state/driving-mode" "PUT" "/api/state/driving-mode" "placeholder_ok" \
  "{\"enabled\":false}" "bearer"

request "deepflowsoc/report" "POST /api/report/global" "POST" "/api/report/global" "must_2xx" \
  "{\"event_id\":\"$EVENT_ID\"}" "bearer"

# Engineer Chat
request "deepflowsoc/engineer-chat" "POST /api/engineer-chat/new-session" "POST" "/api/engineer-chat/new-session" "must_2xx" "{}" "bearer"
request "deepflowsoc/engineer-chat" "GET /api/engineer-chat/status" "GET" "/api/engineer-chat/status?event_id=$EVENT_ID" "must_2xx" "" "bearer"
request "deepflowsoc/engineer-chat" "POST /api/engineer-chat/send" "POST" "/api/engineer-chat/send" "must_2xx" \
  "{\"event_id\":\"$EVENT_ID\",\"message\":\"请给出初步研判\"}" "bearer"
request "deepflowsoc/engineer-chat" "GET /api/engineer-chat/history" "GET" "/api/engineer-chat/history?event_id=$EVENT_ID" "must_2xx" "" "bearer"

# Adapter / internal
request "adapter/internal" "POST /internal/admin/dedup/reset" "POST" "/internal/admin/dedup/reset" "placeholder_ok" "{}" "internal"

LY_PAYLOAD='{
  "id":"compat-ly-001",
  "event_id":"compat-ly-event-001",
  "threat_source":"198.51.100.10",
  "victim_target":"172.16.20.20",
  "rule_desc":"SSH Brute Force",
  "event_type":"Network Threat",
  "detail_type":"SSH",
  "event_level":"高",
  "method":"rule",
  "occurrence_time":"2026-01-01T00:00:00Z",
  "duration":60,
  "is_active":true,
  "processing_status":"new"
}'
request "adapter/internal" "POST /internal/event/push" "POST" "/internal/event/push" "must_2xx" "$LY_PAYLOAD" "internal"
request "adapter/internal" "POST /internal/sync:run" "POST" "/internal/sync:run" "upstream_optional" "{}" "internal"
request "adapter/internal" "GET /internal/flows/{flow_id}" "GET" "/internal/flows/compat-flow-001" "upstream_optional" "" "internal"
request "adapter/internal" "GET /internal/flows/{flow_id}/related" "GET" "/internal/flows/compat-flow-001/related?window=1h&rel_type=src&limit=10" "upstream_optional" "" "internal"
request "adapter/internal" "GET /internal/assets/{ip}" "GET" "/internal/assets/172.16.20.20" "upstream_optional" "" "internal"
request "adapter/internal" "POST /internal/flows/{flow_id}/pcap:prepare" "POST" "/internal/flows/compat-flow-001/pcap:prepare" "placeholder_ok" "{}" "internal"
request "adapter/internal" "GET /internal/pcaps/{pcap_id}" "GET" "/internal/pcaps/compat-pcap-001" "upstream_optional" "" "internal"

# ly_server /d/* 兼容入口；当前通常是代理，502 表示上游未配置，记 WARN
request "ly_server/d" "POST /d/auth" "POST" "/d/auth" "upstream_optional" "{\"username\":\"admin\",\"password\":\"admin\"}"
request "ly_server/d" "GET /d/sctl" "GET" "/d/sctl" "upstream_optional"
request "ly_server/d" "GET /d/event" "GET" "/d/event?req_type=aggre" "upstream_optional"
request "ly_server/d" "GET /d/config" "GET" "/d/config" "upstream_optional"
request "ly_server/d" "POST /d/config" "POST" "/d/config" "upstream_optional" "{}"
request "ly_server/d" "GET /d/mo" "GET" "/d/mo?op=get&moip=172.16.20.20" "upstream_optional"
request "ly_server/d" "POST /d/mo" "POST" "/d/mo" "upstream_optional" "{}"
request "ly_server/d" "GET /d/feature" "GET" "/d/feature?id=compat-flow-001" "upstream_optional"
request "ly_server/d" "GET /d/topn" "GET" "/d/topn" "upstream_optional"
request "ly_server/d" "GET /d/evidence" "GET" "/d/evidence" "upstream_optional"
request "ly_server/d" "GET /d/bwlist" "GET" "/d/bwlist" "upstream_optional"
request "ly_server/d" "POST /d/bwlist" "POST" "/d/bwlist" "upstream_optional" "{}"

# 清理测试用户，失败不影响总流程
if [[ -n "$TEST_USER_ID" ]]; then
  request "cleanup" "DELETE /api/user/{user_id}" "DELETE" "/api/user/$TEST_USER_ID" "placeholder_ok" "" "bearer"
fi

cat >> "$REPORT_FILE" <<EOF

## 汇总

- TOTAL：$TOTAL
- PASS：$PASS
- WARN：$WARN
- FAIL：$FAIL

## 解释

- WARN 中的 \`/d/*\` 或 \`/internal/flows/*\` 多数代表当前仍依赖 ly_server / FlowShadow 上游。
- WARN 中的 202 多数代表接口入口已兼容，但业务逻辑仍是占位或异步接受。
- FAIL 是下一阶段优先补齐项。

EOF

echo
echo "${C_INFO}== 汇总 ==${C_RESET}"
echo "TOTAL=$TOTAL PASS=$PASS WARN=$WARN FAIL=$FAIL"
echo "REPORT=$REPORT_FILE"

if [[ "$FAIL" -gt 0 ]]; then
  exit 1
fi
