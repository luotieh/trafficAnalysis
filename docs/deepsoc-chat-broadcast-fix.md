# DeepSOC Socket.IO REST Broadcast Fix

本补丁修复 REST 写消息后 Socket.IO 房间收不到 `new_message` 的问题。

修复点：

1. 在 `internal/httpapi` 增加响应捕获 wrapper。
2. 将 `POST /api/event/send_message/{event_id}` 包装为写入成功后广播 `new_message`。
3. 将 `POST /api/engineer-chat/send` 包装为写入成功后广播 `new_message`。
4. 将 `POST /api/event/{event_id}/execution/{execution_id}/complete` 包装为写入成功后广播 `execution_update`。
5. 将 `realtime.BroadcastMessage` 的 payload 改为直接广播消息对象，兼容原 DeepSOC 前端监听习惯。
6. 修复 `scripts/test-socketio-broadcast.sh`，按 Socket.IO polling 长轮询语义先挂起 poll，再触发 REST 写消息。

验证：

```bash
gofmt -w internal/httpapi/*.go internal/realtime/*.go internal/socketio/*.go
go test ./...
docker compose -f deploy/docker-compose.yml build --no-cache traffic-go
docker compose -f deploy/docker-compose.yml up -d --force-recreate traffic-go
./scripts/test-socketio-broadcast.sh
```

预期：

```text
PASS: REST message write broadcasts Socket.IO new_message
```
