# 300 人同群瞬时容量报告

## 1. 结论

RelayIM 在当前本机 Docker 环境完成了 300 人群成员同时发送、300 个 WebSocket 同时在线的容量验收。

| 指标 | 结果 |
| --- | --- |
| 群成员与发送用户 | 300 |
| 同时在线 WebSocket | 300 |
| 同秒发送消息 | 300 |
| HTTP Accepted | 300 / 300 |
| HTTP 429 与 5xx | 0 |
| 发送接口 P95 / P99 | 2,930.32 ms / 3,133.49 ms |
| WebSocket 建连成功率 | 100% |
| 完整收到全部 300 条消息的连接 | 300 / 300 |
| 在线投递帧数 | 90,000 |
| 本人消息端到端 P95 / P99 | 6,487.75 ms / 6,657.20 ms |
| 消息持久化与序号 | 300 条，序号连续且唯一 |
| Outbox 积压峰值 | 178 条待发布、64 条处理中 |
| Outbox 死信 | 0 |

该结果证明当前单实例可以承接 300 人同群瞬时发送，并在约 8 秒内完成全部在线扇出。它不代表已经证明“300 个用户每秒各发一条并持续运行”，后者需要单独进行持续吞吐和资源饱和测试。

## 2. 测试环境

| 项目 | 配置 |
| --- | --- |
| CPU | AMD Ryzen 7 5800H |
| 逻辑核心 | 16 |
| WSL2 内存 | 15 GiB |
| Docker 内存上限 | 15.54 GiB |
| Go | 1.26.11 |
| k6 | 0.57.0 |
| MySQL | 8.0.34 |
| RabbitMQ | 4.3.7 |
| Redis | 7.4 |

测试由独立的 300 个压测账号加入同一群。每个账号先建立 WebSocket，再在统一时刻通过真实 HTTP 消息接口发送一条消息，不使用测试专用写接口绕过认证、事务、Outbox 或 RabbitMQ。

## 3. 调优前后

初始配置的 WebSocket Ticket 全局上限为 250 次/秒。300 个连接集中建立时产生了 50 次 `429`，客户端按 `Retry-After` 重试后最终建连成功，但这说明配置边界与 500 人群上限不匹配。

调整后：

- WebSocket Ticket 全局上限提升到 500 次/秒，与单群最大成员数对齐。
- 消息发送全局配额提升到 1,000 条/10 秒，为 300 人群瞬时发送和其他业务消息保留余量。
- 单用户消息配额保持 20 条/10 秒，不因全局扩容放宽单账号滥用边界。
- 领域事件 Consumer 从 1 个调整为可配置的 4 个，每个 Consumer 使用独立 RabbitMQ 连接和 Publisher Confirm 通道。

最终有效复测中，发送和 Ticket 均未触发 `429`。300 人基线场景的本人消息端到端 P95 从 8,088.70 ms 降到 6,487.75 ms，全部消息仍保持 100% 在线覆盖。

## 4. 资源峰值

| 指标 | 峰值 |
| --- | --- |
| 应用容器 CPU | 120.10% |
| 应用容器内存 | 89.62 MiB |
| Go 堆内存 | 44.45 MiB |
| Goroutine | 1,331 |
| MySQL 打开连接 | 33 |
| MySQL 使用中连接 | 27 |
| WebSocket 连接 | 301 |

监控峰值为 301 个连接，容量判定以 k6 创建的 300 个测试会话为准，额外一条来自监控期间已有的本地客户端。

## 5. 可靠性核对

- 300 条消息全部返回 `201` 和有效 `seq`。
- 最新 300 条消息的 `seq` 为连续区间，没有重复或缺口。
- 每一条消息都投递到全部 300 个在线连接，共 90,000 次帧入队。
- Outbox 最终 `pending=0`、`processing=0`、`dead=0`。
- 客户端仍以 MySQL 中的 `seq` 作为最终补拉依据，在线帧不承担持久化职责。

## 6. 容量边界

同群消息严格串行分配 `seq`，因此同一群的瞬时发送成本包含会话行锁排队。当前结果适合描述为：

> 单实例、300 人群、300 个在线连接、同秒各发一条，发送成功率 100%，完整在线覆盖 100%，端到端 P99 约 6.7 秒。

不应描述为：

> 已支持 300 条/秒持续消息吞吐。

后续若需要持续高吞吐，应增加长时间阶梯压测，再根据 RabbitMQ 队列深度、MySQL 行锁等待和容器 CPU 判断是否需要分片、批量确认或独立的更大消费者资源池。

## 7. 复现

生成 300 人夹具：

```powershell
go run .\loadtest\cmd\groupfixture `
  -members 300 `
  -output .tmp/group-capacity-300.json
```

执行容量测试：

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

测试完成后使用 `loadtest/cleanup-test-users.sql` 清理 `load_gc_*` 账号及其依赖数据。

原始结果：

- [300 人 k6 摘要](./group-capacity-300-final.json)
- [300 人资源峰值摘要](./group-capacity-300-verified-monitor-summary.json)
- [300 人资源采样](./group-capacity-300-verified-monitor.csv)
- [200 人调优前基线](./group-capacity-200-baseline.json)
- [300 人调优前基线](./group-capacity-300-baseline.json)
