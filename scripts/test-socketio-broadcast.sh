#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:9010}"
EVENT_ID="${EVENT_ID:-demo-event-001}"
USERNAME="${ADMIN_USERNAME:-admin}"
PASSWORD="${ADMIN_PASSWORD:-admin123}"

echo "== Socket.IO REST broadcast compatibility test =="
echo "BASE_URL=$BASE_URL EVENT_ID=$EVENT_ID"

python3 - <<PY
import json
import socket
import threading
import time
import urllib.parse
import urllib.request

BASE_URL = "${BASE_URL}".rstrip("/")
EVENT_ID = "${EVENT_ID}"
USERNAME = "${USERNAME}"
PASSWORD = "${PASSWORD}"

def req(method, url, data=None, headers=None, timeout=10):
    headers = headers or {}
    body = None
    if data is not None:
        if isinstance(data, (dict, list)):
            body = json.dumps(data).encode("utf-8")
            headers.setdefault("Content-Type", "application/json")
        elif isinstance(data, str):
            body = data.encode("utf-8")
            headers.setdefault("Content-Type", "text/plain;charset=UTF-8")
        else:
            body = data
    r = urllib.request.Request(url, data=body, headers=headers, method=method)
    try:
        with urllib.request.urlopen(r, timeout=timeout) as resp:
            return resp.status, resp.read().decode("utf-8", errors="replace")
    except (socket.timeout, TimeoutError):
        return 599, ""
    except urllib.error.HTTPError as e:
        return e.code, e.read().decode("utf-8", errors="replace")

def socket_url(sid=None):
    q = {
        "EIO": "4",
        "transport": "polling",
        "t": str(int(time.time() * 1000)),
    }
    if sid:
        q["sid"] = sid
    return BASE_URL + "/socket.io/?" + urllib.parse.urlencode(q)

def socket_get(sid, timeout=30):
    status, body = req("GET", socket_url(sid), timeout=timeout)
    return status, body

def socket_post(sid, packet):
    status, body = req("POST", socket_url(sid), data=packet, timeout=10)
    if status not in (200, 204):
        raise SystemExit(f"Socket.IO POST failed: status={status} body={body!r}")
    return body

status, body = req("GET", socket_url(), timeout=10)
print("handshake:", status, body)
if status != 200 or not body.startswith("0"):
    raise SystemExit("FAIL: Socket.IO handshake failed")

sid = json.loads(body[1:])["sid"]
print("sid=", sid)

print("== namespace connect ==")
print(socket_post(sid, "40"))
status, body = socket_get(sid, timeout=10)
print("connect poll:", status, repr(body))
if "40" not in body:
    raise SystemExit("FAIL: Socket.IO namespace connect ack missing")

print("== join room ==")
join_packet = '42["join",{"event_id":"' + EVENT_ID + '"}]'
print(socket_post(sid, join_packet))

received = {"ok": False, "data": ""}

def poll_loop():
    deadline = time.time() + 45
    chunks = []
    while time.time() < deadline:
        status, part = socket_get(sid, timeout=15)
        if part:
            print("poll:", repr(part))
            chunks.append(part)
            if "new_message" in part:
                received["ok"] = True
                received["data"] = "".join(chunks)
                return
        else:
            print("poll timeout/empty:", status)
    received["data"] = "".join(chunks)

poller = threading.Thread(target=poll_loop, daemon=True)
poller.start()
time.sleep(0.5)

print("== login ==")
status, login_body = req(
    "POST",
    BASE_URL + "/api/auth/login",
    data={"username": USERNAME, "password": PASSWORD},
    timeout=10,
)
print("login:", status, login_body)
if status != 200:
    raise SystemExit("FAIL: login failed")

login = json.loads(login_body)
token = login.get("access_token") or login.get("data", {}).get("access_token") or login.get("token")
if not token:
    raise SystemExit("FAIL: access_token missing")

print("== send REST message ==")
message = "Socket.IO broadcast compatibility test " + str(int(time.time()))
status, send_body = req(
    "POST",
    BASE_URL + "/api/event/send_message/" + EVENT_ID,
    data={"message": message, "content": message, "message_content": message},
    headers={"Authorization": "Bearer " + token},
    timeout=10,
)
print("send:", status, send_body)
if status != 200:
    raise SystemExit("FAIL: REST send message failed")

poller.join(timeout=50)

if not received["ok"]:
    raise SystemExit("FAIL: did not receive new_message after REST send; poll=" + repr(received["data"]))

print("PASS: REST message write broadcasts Socket.IO new_message")
PY
