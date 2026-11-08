# HTTP 消息基线报告

## 1. 测试日期

2026-11-08

## 2. 测试目标

验证单实例 GinChat 在固定并发下：

- 消息 HTTP 请求是否稳定返回 `201 Created`。
- 服务端是否返回有效会话序号。
- 消息 Accepted 延迟和错误率。
- RabbitMQ 发布后 Outbox 是否出现持续积压或死信。

本报告只覆盖 HTTP 消息 Accepted 链路，不代表 WebSocket 端到端在线投递延迟。

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
| 对象存储 | Docker Desktop WSL2，SeaweedFS 4.52 |
| 压测工具 | Docker 容器 `grafana/k6:0.57.0` |

所有组件与压测客户端运行在同一台开发机上，结果适合本地基线和简历演示，不等同于独立服务器生产压测。

## 4. 场景参数

| 参数 | 值 |
| --- | --- |
| Executor | `constant-vus` |
| 虚拟用户数 | 10 VU |
| 持续时间 | 30 秒 |
| 每轮等待 | 0.1 秒 |
| 消息类型 | 文本 |
| 会话 | 一组独立私聊 |
| 幂等键 | 每条消息唯一 |

脚本会先创建两个独立账号、建立好友关系、创建私聊，然后并发发送消息。

## 5. 测试结果

| 指标 | 结果 |
| --- | ---: |
| 成功消息数 | 2,696 |
| HTTP 请求总数 | 2,704 |
| HTTP 请求速率 | 89.04 req/s |
| 消息处理速率 | 88.77 条/s |
| HTTP 失败率 | 0.00% |
| 消息接口 P50 | 9.26 ms |
| 消息接口 P90 | 12.41 ms |
| 消息接口 P95 | 15.25 ms |
| 消息接口 P99 | 40.67 ms |
| 最大请求耗时 | 73.36 ms |
| 检查通过率 | 100% |

结果满足脚本阈值：

```text
checks > 99%
http_req_failed < 1%
消息接口 P95 < 1000 ms
消息接口 P99 < 2000 ms
```

## 6. Outbox 观察

压测消息发送完成并等待 RabbitMQ 发布后：

| 状态 | 数量 |
| --- | ---: |
| pending | 0 |
| processing | 0 |
| dead | 0 |

本次并发水平下未观察到持续积压、卡在处理中的租约或死信。

## 7. 结论

- 当前单实例在 10 VU、约 89 条消息/秒的短时负载下没有 HTTP 错误。
- 消息接口 P99 为 40.67 ms，数据库事务、连续序号分配和 Outbox 写入未成为本次负载下的明显瓶颈。
- RabbitMQ 能够跟上本次测试的发布速度，未形成可观察积压。
- 该结果不能用于声称“支持 5,000 连接”或生产级吞吐；这些需要独立服务器、长稳和 WebSocket 场景验证。

## 8. 已知限制

- 发送者和接收者通过同一台本机服务运行，未覆盖真实公网 RTT。
- 未测量 WebSocket 端到端在线投递延迟。
- 未测试慢客户端、断线重连、消费者故障和 RabbitMQ DLQ。
- 未进行 30 分钟以上长稳测试。
- 未覆盖图片和文件上传吞吐。
- 所有基础设施与压测客户端共享同一台电脑，CPU、磁盘和网络可能相互影响。

## 9. 复现

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

安全摘要：[http-baseline.json](./http-baseline.json)
