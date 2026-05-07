# DeepSOC Driving Mode Automation

本补丁补齐原 DeepSOC 自动驾驶模式的最小闭环：

1. `GET /api/state/driving-mode` 从 PostgreSQL 持久化状态读取。
2. `PUT /api/state/driving-mode` 将 `enabled` 写入 `app_states`。
3. 创建事件后，如自动驾驶已开启，自动生成：
   - task
   - execution
   - summary
   - autopilot system message
4. 自动驾驶执行过程中通过 Socket.IO 推送：
   - `new_message`
   - `execution_update`
   - `status`

## 数据表

新增 PostgreSQL 表：

```sql
CREATE TABLE IF NOT EXISTS app_states (
    key VARCHAR(128) PRIMARY KEY,
    value JSONB NOT NULL DEFAULT '{}'::jsonb,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

`driving_mode` 状态示例：

```json
{"enabled": true}
```

## 验证

```bash
gofmt -w internal/autopilot/*.go internal/httpapi/*.go
go test ./...
docker compose -f deploy/docker-compose.yml build --no-cache traffic-go
docker compose -f deploy/docker-compose.yml up -d --force-recreate traffic-go
./scripts/test-driving-mode-compat.sh
```

预期：

```text
PASS: driving mode persisted enabled=true
PASS: driving mode triggers task/execution/summary/message automation artifacts.
```

## 说明

本补丁实现的是自动驾驶最小可用闭环。后续可继续接入真实 LLM、证据抓取、PCAP 生成、执行器插件和复杂任务编排。
