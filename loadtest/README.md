# 压测

当前提供 HTTP 消息基线脚本：

```text
loadtest/k6/http-message.js
loadtest/k6/group-capacity.js
```

## 场景

脚本自动完成：

1. 创建一对独立压测账号。
2. 发送并接受好友申请。
3. 创建唯一私聊。
4. 使用固定 VU 并发发送带唯一 `client_message_id` 的文本消息。
5. 校验消息返回 `201` 且具有服务端 `seq`。

该场景只测消息 Accepted 链路，不包含 WebSocket 空闲连接和端到端在线投递延迟。

## 同群容量场景

`group-capacity.js` 用于验证 200 至 500 人同群瞬时发送：

1. 所有成员先建立 WebSocket。
2. 所有成员在统一时刻分别通过真实 HTTP 接口发送一条消息。
3. 每个在线连接统计是否完整收到全部群消息。
4. 记录 Accepted、`429`、P95/P99、在线扇出覆盖和本人消息端到端延迟。

夹具由 `loadtest/cmd/groupfixture` 直接创建本地压测账号、设备和群成员。夹具进程必须读取与正在运行应用相同的 JWT、MySQL 和 Redis 配置。先确认目标账号前缀 `load_gc_` 没有被业务数据使用。

生成 300 人夹具：

```powershell
go run .\loadtest\cmd\groupfixture `
  -members 300 `
  -output .tmp\group-capacity-300.json
```

执行测试：

```powershell
docker run --rm `
  --add-host=host.docker.internal:host-gateway `
  -e BASE_URL=http://host.docker.internal:8081 `
  -e FIXTURE_FILE=../../.tmp/group-capacity-300.json `
  -e SEND_START_SECONDS=12 `
  -e DELIVERY_WAIT_SECONDS=60 `
  -e SUMMARY_FILE=group-capacity-300-final.json `
  -v "${PWD}:/work" `
  -w /work `
  grafana/k6:0.57.0 run loadtest/k6/group-capacity.js
```

同步采集应用和容器资源：

```powershell
.\loadtest\monitor-soak.ps1 `
  -DurationSeconds 130 `
  -IntervalSeconds 2 `
  -OutputPath loadtest\reports\group-capacity-monitor.csv `
  -SummaryPath loadtest\reports\group-capacity-monitor-summary.json
```

## 本地运行

使用固定版本 k6 容器：

```powershell
docker run --rm `
  --add-host=host.docker.internal:host-gateway `
  -e BASE_URL=http://host.docker.internal:8081 `
  -e VUS=10 `
  -e DURATION=30s `
  -e SUMMARY_FILE=http-baseline.json `
  -v "${PWD}:/work" `
  -w /work `
  grafana/k6:0.57.0 run loadtest/k6/http-message.js
```

可用参数：

| 环境变量 | 默认值 | 说明 |
| --- | --- | --- |
| `BASE_URL` | `http://127.0.0.1:8081` | Go 服务地址 |
| `VUS` | `10` | 并发虚拟用户数 |
| `DURATION` | `30s` | 压测持续时间 |
| `SLEEP_SECONDS` | `0.8` | 每轮请求后的等待时间；每个 VU 使用独立发送账号，避免单用户限流影响吞吐结果 |
| `SUMMARY_FILE` | `http-baseline.json` | 安全摘要文件名；不包含 Token 和业务 ID |

## 结果解释

- `http_req_duration{endpoint:send_message}`：消息 HTTP 接口耗时，不等同于 WebSocket 端到端投递延迟。
- `http_req_failed`：HTTP 失败率。
- `relayim_message_accepted_total`：服务端成功持久化的消息数。
- `relayim_outbox_events`：压测过程中 Outbox 是否积压。

压测账号使用 `load_http_*` 等固定前缀，动态 E2E 也使用独立测试前缀。测试结束必须清理，不能把测试消息和 Outbox 事件留在开发库。

确认没有压测正在运行后，可执行：

```powershell
Get-Content -Raw loadtest\cleanup-test-users.sql |
  docker exec -i relayim-mysql sh -c `
    'MYSQL_PWD="$MYSQL_ROOT_PASSWORD" mysql -uroot relayim'
```

## 已有报告

- [30 分钟长稳、容量与故障注入：2026-11-12](./reports/soak-fault-2026-11-12.md)
- [300 人同群瞬时容量：2026-11-12](./reports/group-capacity-2026-11-12.md)
- [代码审查与压测：2026-11-11](./reports/audit-2026-11-11.md)
- [HTTP 消息基线：10 VU / 30 秒](./reports/http-baseline.md)
- [WebSocket 投递基线：20 个在线连接 / 30 秒](./reports/websocket-baseline.md)
- [WebSocket 空闲连接阶梯：100 / 500 / 1,000 / 2,000 / 5,000](./reports/websocket-connections.md)
- [WebSocket Ticket 限流验证](./reports/ticket-rate-limit.md)

WebSocket 连接与投递基线使用：

```powershell
docker run --rm `
  --add-host=host.docker.internal:host-gateway `
  -e BASE_URL=http://host.docker.internal:8081 `
  -e WS_RECEIVERS=20 `
  -e DURATION_SECONDS=30 `
  -e SUMMARY_FILE=websocket-baseline.json `
  -v "${PWD}:/work" `
  -w /work `
  grafana/k6:0.57.0 run loadtest/k6/websocket-delivery.js
```

空闲连接阶梯测试使用：

```powershell
docker run --rm `
  --add-host=host.docker.internal:host-gateway `
  -e BASE_URL=http://host.docker.internal:8081 `
  -e WS_CONNECTIONS=100 `
  -e HOLD_SECONDS=20 `
  -e CONNECT_RAMP_SECONDS=5 `
  -e SUMMARY_FILE=websocket-connections-100.json `
  -v "${PWD}:/work" `
  -w /work `
  grafana/k6:0.57.0 run loadtest/k6/websocket-connections.js
```

Ticket 请求遇到 `429 Too Many Requests` 时，脚本会读取 `Retry-After` 并加入随机抖动后重试。`TICKET_RETRY_ATTEMPTS` 默认 120，用于验证限流后的流量恢复，而不是绕过限流。

## 30 分钟长稳

以下命令使用 20 个独立发送账号、20 个 VU，每个账号约每秒发送一条消息：

```powershell
docker run --rm `
  --add-host=host.docker.internal:host-gateway `
  -e BASE_URL=http://host.docker.internal:8081 `
  -e VUS=20 `
  -e DURATION=30m `
  -e SLEEP_SECONDS=1 `
  -e SUMMARY_FILE=soak-http-2026-11-12.json `
  -v "${PWD}:/work" `
  -w /work `
  grafana/k6:0.57.0 run loadtest/k6/http-message.js
```

另开终端采集应用和容器资源：

```powershell
.\loadtest\monitor-soak.ps1 `
  -DurationSeconds 2280 `
  -IntervalSeconds 5
```

脚本输出 CSV、资源峰值和首尾消息/消费计数差值。

## 基础设施故障注入

以下脚本会短暂停止并自动恢复 RabbitMQ、Redis 和 MySQL：

```powershell
python scripts\infrastructure-fault-e2e.py
```

执行前确认没有其他压测或人工操作依赖这些容器。测试覆盖：

- RabbitMQ 中断期间消息写入 Outbox，恢复后队列排空且 DLQ 不新增。
- Redis 中断时认证和写请求快速返回 `503`，恢复后重新登录和发送成功。
- MySQL 中断时写请求失败、就绪检查返回 `503`，恢复后继续发送。
