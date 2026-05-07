# DeepSOC Socket.IO 聊天室兼容实现

本补丁为 Go 重构版增加 `/socket.io/` 入口，用于兼容原 DeepSOC Flask-SocketIO 作战室聊天室。

## 支持内容

- Engine.IO v4 polling handshake
- Socket.IO websocket transport
- Socket.IO 基础事件：
  - `join`
  - `leave`
  - `message`
  - `test_connection`
- 服务端推送事件：
  - `status`
  - `test_message`
  - `test_connection_response`
  - `new_message`

## 前端兼容

原前端如果使用 `socket.io-client`，连接地址应指向 Go 后端：

```js
const socket = io('http://10.20.30.113:9010')
```

或走前端代理时确保 `/socket.io/` 被代理到 `http://10.20.30.113:9010`。

Vite 示例：

```js
server: {
  proxy: {
    '/api': { target: 'http://10.20.30.113:9010', changeOrigin: true },
    '/socket.io': { target: 'http://10.20.30.113:9010', ws: true, changeOrigin: true }
  }
}
```

## 验证

```bash
go mod tidy
go mod vendor
gofmt -w internal/socketio/*.go internal/httpapi/server.go
go test ./...
./scripts/reset-demo.sh
./scripts/test-socketio-compat.sh
```

## 注意

当前版本先完成 Socket.IO 协议兼容和房间广播。REST 接口写入消息后的统一广播钩子可在下一步接入，将 `POST /api/event/send_message/{event_id}`、`POST /api/engineer-chat/send`、RabbitMQ worker 写入消息后统一调用 socket hub 广播。
