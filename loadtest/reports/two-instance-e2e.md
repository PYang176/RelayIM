# 两实例实时投递验证

## 1. 测试日期

2026-11-09

## 2. 测试环境

| 项目 | 配置 |
| --- | --- |
| CPU | AMD Ryzen 7 5800H，8 核 16 线程 |
| 内存 | 32 GB |
| Go 实例 1 | `127.0.0.1:18081`，Snowflake Worker 1 |
| Go 实例 2 | `127.0.0.1:18082`，Snowflake Worker 2 |
| MySQL | 本机 MySQL 8.0.34 |
| Redis | Docker，Redis 7.4 |
| RabbitMQ | Docker，RabbitMQ 4.3.7 |
| 客户端 | Node.js 24 原生 `WebSocket` |

两个 Go 实例共享同一 MySQL、Redis、RabbitMQ 和对象存储。

## 3. 场景

| 连接 | 用户设备 | 实例 |
| --- | --- | --- |
| 1 | Alice 设备 1 | 实例 1 |
| 2 | Alice 设备 2 | 实例 2 |
| 3 | Bob 设备 1 | 实例 2 |

Alice 设备 1 发送一条私聊消息，随后在同一会话撤回该消息。

## 4. 结果

```json
{
  "delivery_latency_ms": 559,
  "delivered_to": 3,
  "recalled_on": 3,
  "sender_device_on_instance_1": true,
  "recipient_on_instance_2": true,
  "same_user_second_device": true
}
```

验证完成后：

- Redis 测试用户实时路由键为 0。
- 两个 `im.delivery.node.*` 队列均为 0 积压。
- 两个队列各有 1 个本实例 Consumer。

## 5. 说明

本次结果是单条功能验证，不是吞吐压测。559 ms 包含 Outbox 默认 500 ms 轮询等待，因此不能直接作为 WebSocket 网络延迟。跨实例容量、故障切换和 30 分钟长稳仍需独立压测。
