# DeepSOC 聊天室实时广播钩子

本补丁在已有 `/socket.io/` 兼容层基础上补充广播钩子，目标是让 REST/API/Worker 写入消息后，前端聊天室能实时收到 `new_message`。

## 设计

新增 `internal/realtime` 轻量发布接口：

- `Register(Publisher)`：Socket.IO Hub 启动时注册全局发布器。
- `Broadcast(room, event, payload)`：向指定房间广播事件。
- `BroadcastMessage(eventID, message)`：广播 DeepSOC 聊天消息，事件名为 `new_message`。
- `BroadcastExecutionUpdate(eventID, payload)`：广播执行状态，事件名为 `execution_update`。

为了避免 REST handler、worker 和 store 层直接依赖具体 Socket.IO 实现，业务路径只依赖 `internal/realtime`。

## 当前接入点

补丁会在 `Store.AddMessage` 成功写入后广播：

```text
messages 表写入成功 -> realtime.BroadcastMessage(event_id, message)
```

因此以下路径会自动获得实时推送能力：

- `POST /api/event/send_message/{event_id}`
- `POST /api/engineer-chat/send`
- 系统消息写入
- RabbitMQ worker 消费后写入 messages 表

前端加入对应 `event_id` 房间后，可监听：

```js
socket.on('new_message', handler)
```

## 验证

```bash
go test ./...
./scripts/reset-demo.sh
./scripts/test-original-api-compat.sh
./scripts/test-socketio-compat.sh
./scripts/test-socketio-broadcast.sh
```
