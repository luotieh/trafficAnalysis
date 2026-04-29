# traffic-go-db

这是 `luotieh/trafficAnalysis` Go 重构骨架的数据库持久化版本。

本版本做了这些改变：

- 新增 `STORE_BACKEND=postgres`。
- 新增 `internal/store/postgres.go`，用 `database/sql + github.com/lib/pq` 实现持久化存储。
- 新增 `internal/store/schema.go`，服务启动时可自动建表。
- 保留 `STORE_BACKEND=memory`，便于本地快速演示。
- 增加 PostgreSQL 版 `docker-compose.yml`。
- 继续保留原三套接口：
  - 原 `adapter` 的 `/internal/*`
  - 原 `deepflowsoc` 的 `/api/*`
  - 原 `ly_server` 的 `/d/*` 代理兼容入口

## 快速启动 PostgreSQL 版

```bash
cp .env.example .env
docker compose -f deploy/docker-compose.yml up -d postgres
export $(grep -v '^#' .env | xargs)
go mod tidy
go run ./cmd/traffic-api
```

健康检查：

```bash
curl http://localhost:9010/healthz
```

登录默认管理员：

```bash
curl -X POST http://localhost:9010/api/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"admin"}'
```

创建事件：

```bash
curl -X POST http://localhost:9010/api/event/create \
  -H 'Content-Type: application/json' \
  -d '{"event_name":"数据库持久化测试","message":"检测到异常流量","severity":"high","source":"manual"}'
```

查看事件：

```bash
curl http://localhost:9010/api/event/list
```

重启服务后再查 `/api/event/list`，事件仍然存在，说明数据库持久化生效。

## 环境变量

| 变量 | 说明 |
|---|---|
| `STORE_BACKEND` | `postgres` 或 `memory` |
| `DATABASE_URL` | PostgreSQL DSN |
| `AUTO_MIGRATE` | `true` 时启动自动建表 |
| `INTERNAL_API_KEY` | `/internal/*` 内部接口密钥 |
| `FLOWSHADOW_BASE_URL` | 原 ly_server/FlowShadow 地址 |
| `DEEPSOC_BASE_URL` | 原 DeepSOC 地址；为空时使用本地 Go 数据库实现 |
| `LLM_BASE_URL` | OpenAI-compatible `/chat/completions` 地址 |

## 与原接口兼容情况

路径层面已覆盖：

- `adapter`: 9 个入口
- `deepflowsoc`: 33 个 path / 38 个 method
- `ly_server`: 9 个 path / 12 个 method，通过 `/d/*` 代理

数据库版新增持久化：

- `users`
- `events`
- `messages`
- `tasks`
- `executions`
- `summaries`
- `event_maps`
- `sync_cursors`
- `pushed_events`
- `audit_logs`
- `prompts`
- `settings`

## 生产化后续建议

1. 密码从当前兼容存储改为 bcrypt。
2. JWT token 存储增加过期时间和刷新逻辑。
3. `tasks / executions / summaries` 接入真实 Agent 编排。
4. `/d/event /d/evidence /d/feature /d/topn` 从代理逐步迁移成 Go 原生查询。
5. 为所有原 OpenAPI 增加自动化兼容测试。

## RabbitMQ 消息队列版本

本版本已把消息队列纳入 Docker 和 Go 代码：

- Docker Compose 同时包含 PostgreSQL、RabbitMQ、Go 服务。
- Go 服务通过 `MQ_BACKEND=rabbitmq` 开启消息投递。
- 创建事件、接入 FlowShadow 事件、同步完成时会发布 MQ 消息。
- 内置基础 worker 会消费队列并向事件消息表写入队列通知。

启动全部服务：

```bash
docker compose -f deploy/docker-compose.yml up -d --build
```

RabbitMQ 管理台：

```text
http://localhost:15672
traffic / traffic
```

只启动基础设施：

```bash
docker compose -f deploy/docker-compose.yml up -d postgres rabbitmq
```

本地运行 Go：

```bash
cp .env.example .env
export $(grep -v '^#' .env | xargs)
go mod tidy
go run ./cmd/traffic-api
```

更多说明见：`docs/mq-refactor.md`。
