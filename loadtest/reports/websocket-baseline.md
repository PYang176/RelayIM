# WebSocket 投递基线报告

## 1. 测试日期

2026-11-08

## 2. 测试目标

验证单实例 RelayIM 在 20 个在线 WebSocket 接收连接下：

- 一次性 Ticket 建连是否稳定。
- 群消息通过 RabbitMQ Consumer 到在线连接的非阻塞扇出。
- 客户端 ACK 和 Read 回执是否持续写入。
- 消息从 HTTP Accepted 到 WebSocket 接收端的时间。
- 测试结束后连接和 Outbox 是否及时回收。

## 3. 测试环境

| 项目 | 配置 |
| --- | --- |
| CPU | AMD Ryzen 7 5800H，8 核 16 线程 |
| 内存 | 32 GB |
| 操作系统 | Windows 11 家庭版，10.0.26200 |
| Go 服务 | Windows 本机直接运行 |
| MySQL | Windows 本机 MySQL 8.0.34 |
| Redis | Docker Desktop WSL2，Redis 7.4 |
| RabbitMQ | Docker Desktop WSL2，RabbitMQ 4.3.7 |
| 压测工具 | Docker 容器 `grafana/k6:0.57.0` |

压测客户端、Go 服务和基础设施运行在同一台电脑上。

## 4. 场景参数

| 参数 | 值 |
| --- | --- |
| 在线接收者 | 20 |
| 发送者 | 1 |
| 总持续时间 | 30 秒 |
| 发送开始时间 | 第 3 秒 |
| 发送持续时间 | 24 秒 |
| 发送间隔 | 0.2 秒 |
| 会话类型 | 21 人群聊 |

每个接收 VU 都会：

1. 使用独立账号登录。
2. 申请一次性 WebSocket Ticket。
3. 建立并保持一个 WebSocket 连接。
4. 收到群消息后发送 ACK 和 Read。
5. 根据消息正文中的发送时间计算端到端延迟。

## 5. 测试结果

| 指标 | 结果 |
| --- | ---: |
| WebSocket 连接成功 | 20 / 20 |
| 连接成功率 | 100% |
| 发送消息 | 114 条 |
| WebSocket 消息投递 | 2,280 次 |
| 投递速率 | 69.18 次/秒 |
| HTTP 失败率 | 0.00% |
| 检查通过率 | 100% |
| 端到端平均延迟 | 267.98 ms |
| 端到端 P50 | 263.5 ms |
| 端到端 P90 | 464 ms |
| 端到端 P95 | 482 ms |
| 端到端 P99 | 500 ms |
| 最大延迟 | 521 ms |

结果满足预先设定阈值：

```text
WebSocket 建连成功率 > 99%
投递 P95 < 500 ms
投递 P99 < 1000 ms
HTTP 失败率 < 1%
```

## 6. 延迟分析

消息投递延迟中位数约 263.5 ms，主要分布在 0 至 500 ms 区间。该分布与当前 Outbox 默认 500 ms 轮询间隔高度相关，说明端到端耗时主要包含：

1. HTTP 消息事务与 Outbox 写入。
2. Outbox 等待下一轮发布。
3. RabbitMQ Consumer 处理和 WebSocket 有界队列投递。

本次没有单独的 Outbox 等待耗时指标，因此该解释是基于轮询配置与延迟分布的相关性，不能替代下一阶段的链路级延迟拆解。

## 7. 清理观察

测试结束后：

| 项目 | 状态 |
| --- | ---: |
| 当前 WebSocket 连接 | 0 |
| Outbox pending | 0 |
| Outbox processing | 0 |
| Outbox dead | 0 |

测试账号、群聊、消息、送达回执、Outbox 和 Redis Token 已清理。

## 8. 结论

- 当前单实例可以稳定支持 20 个同时在线 WebSocket 接收连接。
- 21 人群聊、20 个在线接收者、约 5 条消息/秒的发送速率下，消息全部完成扇出。
- ACK 和 Read 回执没有导致失败或阻塞。
- P95 482 ms 满足 500 ms 目标，但离阈值较近，连接规模和发送速率继续提高前需要链路级延迟拆解。
- 该结果不能用于声称“支持 5,000 连接”或生产级容量。

## 9. 已知限制

- 仅 20 个在线连接，距离需求中的 5,000 空闲连接目标很远。
- 所有组件和压测客户端共用一台电脑。
- 未模拟慢客户端、连接抖动、网络丢包和断线重连。
- 未停止 RabbitMQ Consumer 或注入 DLQ 故障。
- 未进行 30 分钟以上长稳测试。
- 端到端延迟是一个整体值，尚未拆分为 Outbox 等待、Broker、Consumer 和写队列耗时。

## 10. 复现

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

安全摘要：[websocket-baseline.json](./websocket-baseline.json)
