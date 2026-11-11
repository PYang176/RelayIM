# 压测报告索引

所有结果来自本地 Docker 开发环境，用于功能回归、容量基线和故障恢复验证，不代表生产集群上限。汇总结论和统一口径见 [性能与容量验收](../../docs/性能与容量验收.md)。

## 1. 核心报告

| 报告 | 内容 | 原始数据 |
| --- | --- | --- |
| [30 分钟长稳、容量与故障注入](./soak-fault-2026-11-12.md) | HTTP 长稳、40 VU 容量和基础设施故障恢复 | [长稳 JSON](./soak-http-2026-11-12.json)、[容量 JSON](./capacity-http-2026-11-12.json)、[故障 JSON](./fault-injection-2026-11-12.json) |
| [300 人同群瞬时容量](./group-capacity-2026-11-12.md) | 300 人同秒发送、300 个在线连接和 90,000 次扇出 | [k6 摘要](./group-capacity-300-final.json)、[资源峰值](./group-capacity-300-verified-monitor-summary.json) |
| [WebSocket 连接阶梯](./websocket-connections.md) | 100、500、1,000、2,000、5,000 连接 | [连接报告 JSON](./websocket-connections-5000.json) |
| [WebSocket 投递基线](./websocket-baseline.md) | 20 个在线接收端的实时群消息投递 | [投递 JSON](./websocket-baseline.json) |
| [HTTP 消息基线](./http-baseline.md) | 独立账号的消息 Accepted 链路 | [HTTP JSON](./http-baseline.json) |
| [两实例实时投递](./two-instance-e2e.md) | 跨实例定向投递功能验证 | 端到端脚本输出 |
| [Ticket 限流](./ticket-rate-limit.md) | 滑动窗口超限和恢复 | [限流 JSON](./ticket-rate-limit.json) |
| [代码审查与异常压测](./audit-2026-11-11.md) | 边界、失败场景和首轮性能基线 | 对应 JSON 文件 |

## 2. 数据分组

### HTTP 与可靠性

- [http-baseline.json](./http-baseline.json)
- [soak-http-2026-11-12.json](./soak-http-2026-11-12.json)
- [capacity-http-2026-11-12.json](./capacity-http-2026-11-12.json)
- [fault-injection-2026-11-12.json](./fault-injection-2026-11-12.json)
- [soak-monitor-2026-11-12-summary.json](./soak-monitor-2026-11-12-summary.json)
- [soak-monitor-2026-11-12.csv](./soak-monitor-2026-11-12.csv)

### WebSocket

- [websocket-baseline.json](./websocket-baseline.json)
- [websocket-connections-100.json](./websocket-connections-100.json)
- [websocket-connections-500.json](./websocket-connections-500.json)
- [websocket-connections-1000.json](./websocket-connections-1000.json)
- [websocket-connections-2000.json](./websocket-connections-2000.json)
- [websocket-connections-5000.json](./websocket-connections-5000.json)

### 大群容量

- [200 人调优前基线](./group-capacity-200-baseline.json)
- [300 人调优前基线](./group-capacity-300-baseline.json)
- [300 人最终结果](./group-capacity-300-final.json)
- [300 人资源峰值](./group-capacity-300-verified-monitor-summary.json)
- [300 人资源采样](./group-capacity-300-verified-monitor.csv)

## 3. 数据规则

- 原始摘要不记录 Token、密码、用户 ID、设备 ID 或消息正文。
- 报告中的“人数”“连接数”“消息数”和“并发 VU”含义不同，不应直接相加。
- 本地同机测试结果不能直接等同于生产环境容量。
- 被判定无效或口径不完整的测试不进入核心报告。
- 重新生成报告后，同步更新本文索引和 [性能与容量验收](../../docs/性能与容量验收.md)。
