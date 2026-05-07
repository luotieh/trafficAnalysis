#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:9010}"
EVENT_ID="${EVENT_ID:-demo-event-001}"

printf '== Socket.IO polling handshake ==\n'
OPEN=$(curl -fsS "$BASE_URL/socket.io/?EIO=4&transport=polling&t=$(date +%s)")
echo "$OPEN"
SID=$(printf '%s' "$OPEN" | sed -n 's/^0.*"sid":"\([^"]*\)".*/\1/p')
if [ -z "$SID" ]; then
  echo "failed to parse sid" >&2
  exit 1
fi

echo "sid=$SID"

printf '== Socket.IO namespace connect ==\n'
curl -fsS -X POST "$BASE_URL/socket.io/?EIO=4&transport=polling&sid=$SID" --data-binary '40'
echo
CONNECT_ACK=$(curl -fsS "$BASE_URL/socket.io/?EIO=4&transport=polling&sid=$SID")
echo "$CONNECT_ACK"
if ! printf '%s' "$CONNECT_ACK" | grep -q '^40'; then
  echo "connect ack not received" >&2
  exit 1
fi

printf '== join room ==\n'
JOIN='42["join",{"event_id":"'"$EVENT_ID"'"}]'
curl -fsS -X POST "$BASE_URL/socket.io/?EIO=4&transport=polling&sid=$SID" --data-binary "$JOIN"
echo
JOIN_EVENTS=$(curl -fsS "$BASE_URL/socket.io/?EIO=4&transport=polling&sid=$SID")
echo "$JOIN_EVENTS"
if ! printf '%s' "$JOIN_EVENTS" | grep -q '"status":"joined"'; then
  echo "join status not received" >&2
  exit 1
fi

printf '== test_connection ==\n'
TEST='42["test_connection",{"event_id":"'"$EVENT_ID"'","timestamp":"'"$(date -Is)"'"}]'
curl -fsS -X POST "$BASE_URL/socket.io/?EIO=4&transport=polling&sid=$SID" --data-binary "$TEST"
echo
TEST_EVENTS=$(curl -fsS "$BASE_URL/socket.io/?EIO=4&transport=polling&sid=$SID")
echo "$TEST_EVENTS"
if ! printf '%s' "$TEST_EVENTS" | grep -q 'test_connection_response'; then
  echo "test_connection_response not received" >&2
  exit 1
fi

printf '\nPASS: Socket.IO compatibility endpoint works.\n'
