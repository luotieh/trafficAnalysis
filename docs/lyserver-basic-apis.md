# ly_server 基础管理接口 Go 原生实现

本补丁把以下 `/d/*` 接口从代理改为 Go + PostgreSQL 兼容表直接实现：

- `POST /d/auth` -> `t_user` / `t_user_session`
- `GET /d/sctl` -> `t_agent` / `t_device`
- `GET /d/config` -> `t_config`
- `POST /d/config` -> `t_config`
- `GET /d/mo` -> `t_mo` / `t_mogroup`
- `POST /d/mo` -> `t_mo`
- `GET /d/bwlist` -> `t_blacklist` / `t_whitelist`
- `POST /d/bwlist` -> `t_blacklist` / `t_whitelist`

仍暂时走代理的复杂接口：

- `/d/event`
- `/d/feature`
- `/d/topn`
- `/d/evidence`

验证：

```bash
gofmt -w internal/lyserver/*.go internal/httpapi/server.go
go test ./...
./scripts/reset-demo.sh
./scripts/test-original-api-compat.sh
```
