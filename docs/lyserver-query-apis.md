# ly_server 事件、资产、流量查询接口 Go 原生实现

本补丁把以下接口从代理改为 Go + PostgreSQL 兼容表直接实现：

- `GET /d/event` -> `t_event_data` / `t_event_data_aggre`
- `GET /d/feature` -> `t_event_data`
- `GET /d/topn` -> `t_event_data` 聚合
- `GET /d/evidence` -> `t_event_data` 证据兼容输出
- `GET /internal/assets/{ip}` -> `t_asset_ip` / `t_mo` / `t_asset_srv`
- `GET /internal/flows/{flow_id}` -> `t_event_data`
- `GET /internal/flows/{flow_id}/related` -> `t_event_data` 关联查询
- `GET /internal/pcaps/{pcap_id}` -> 兼容入口，真实 PCAP 文件生成后续补齐

仍需后续深度实现的部分：

- 真实 PCAP 下载/文件管理
- FlowShadow 主动同步 `/internal/sync:run`
- 更完整的 `/d/evidence` 证据链路
- `/internal/admin/dedup/reset` 真正清理去重表

验证：

```bash
gofmt -w internal/lyserver/*.go internal/httpapi/server.go
go test ./...
./scripts/reset-demo.sh
./scripts/test-original-api-compat.sh
```
