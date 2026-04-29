# RabbitMQ 消息队列重构说明

## 为什么要把 RabbitMQ 一起纳入 Docker

数据库解决的是持久化，RabbitMQ 解决的是异步处理、削峰、解耦和失败重试。对本项目来说，以下流程适合异步化：

- FlowShadow/ly_server 事件接入后，异步做证据拉取。
- 事件创建后，异步调用 LLM 分析。
- 任务执行、命令回写、报告生成异步处理。
- 下游 DeepSOC 或外部系统不可用时，避免阻塞 HTTP API。

## 当前实现

本版本新增：

- `internal/mq`：消息队列抽象层，支持 `none` 和 `rabbitmq`。
- `internal/mq/rabbitmq.go`：RabbitMQ 生产者，声明 topic exchange 和 durable queue。
- `internal/worker/rabbit_worker.go`：基础消费者，消费事件消息并写入事件消息表。
- `deploy/docker-compose.yml`：同时启动 PostgreSQL、RabbitMQ、Go 服务。

## 关键配置

```env
MQ_BACKEND=rabbitmq
RABBITMQ_URL=amqp://traffic:traffic@127.0.0.1:5672/
RABBITMQ_EXCHANGE=traffic.events
RABBITMQ_EVENT_QUEUE=traffic.events.default
RABBITMQ_CONSUMER_ENABLED=true
```

如果本地只想调 API，不想启动 RabbitMQ：

```env
MQ_BACKEND=none
```

## 路由键

当前已发布这些消息：

- `event.created`：人工/API 创建事件。
- `event.ingested`：FlowShadow/ly_server 事件接入完成。
- `sync.completed`：同步任务完成。

后续建议扩展：

- `evidence.requested`
- `evidence.ready`
- `llm.analysis.requested`
- `llm.analysis.completed`
- `task.created`
- `task.completed`
- `report.requested`
- `report.completed`

## RabbitMQ 管理台

Docker Compose 启动后访问：

```text
http://localhost:15672
```

账号密码：

```text
traffic / traffic
```

## 一键启动

```bash
docker compose -f deploy/docker-compose.yml up -d postgres rabbitmq
export $(grep -v '^#' .env | xargs)
go run ./cmd/traffic-api
```

或直接容器化启动全部服务：

```bash
docker compose -f deploy/docker-compose.yml up -d --build
```

## 生产化建议

当前版本完成了 MQ 接入和基础消费者，但要达到生产级还建议继续补：

1. Outbox 表，确保数据库写入和消息投递一致。
2. Dead Letter Exchange，处理多次失败的消息。
3. 消费者独立进程化，避免 API 服务和 Worker 资源互相影响。
4. 消费幂等表，防止重复消息导致重复处理。
5. 队列监控、告警、重试次数和延迟队列。
