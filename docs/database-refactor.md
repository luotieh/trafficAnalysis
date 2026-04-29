# 数据库重构说明

## 本次完成

- 引入 `STORE_BACKEND`：
  - `memory`: 原内存模式
  - `postgres`: PostgreSQL 持久化模式
- 新增 `PostgresStore` 实现，与原 `Store` 接口兼容。
- 服务启动时根据 `AUTO_MIGRATE=true` 自动创建表。
- 默认种子管理员：
  - 用户名：`admin`
  - 密码：`admin`

## 表设计

核心业务表：

- `users`
- `events`
- `messages`
- `tasks`
- `executions`
- `summaries`

适配器/同步表：

- `event_maps`
- `sync_cursors`
- `pushed_events`
- `audit_logs`

配置扩展表：

- `prompts`
- `settings`

## 仍需后续补强

- 当前为了兼容原型，密码仍以兼容字段存储；生产应改为 bcrypt。
- Token 仍在进程内存中，重启失效；生产应加 `sessions` 表或标准 JWT。
- `prompt` 的 PUT 已保留入口，但还没有接入 prompts 表更新逻辑。
- `/d/*` 仍为代理，不是 Go 原生流量分析查询。
- `pcap` 还需要补齐 `flow_id -> evidence 参数 -> 下载` 的业务映射。
