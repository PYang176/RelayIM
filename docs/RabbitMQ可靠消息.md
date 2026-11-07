# RabbitMQ 可靠消息

## 1. 当前链路

文本消息发送完成后，MySQL 事务同时提交 `messages`、会话摘要和 `outbox_events`。后台发布器读取 Outbox，通过 RabbitMQ Publisher Confirm 发布 `message.created.v1`，消费者使用手动 ACK 处理并登记消费幂等结果。

```text
POST 消息
  -> MySQL 事务：message + last_seq + outbox
  -> Outbox Worker：领取事件
  -> RabbitMQ im.events
  -> im.message.fanout.v1
  -> Consumer：校验消息事实 + processed_events
  -> ACK
```

接口成功响应只表示数据库事务已经提交，不表示 RabbitMQ 已完成投递。项目采用至少一次投递，消费端必须按 `event_id` 幂等。

## 2. 拓扑

| 名称 | 类型 | 用途 |
| --- | --- | --- |
| `im.events` | Durable Topic Exchange | 业务领域事件 |
| `im.dlx` | Durable Direct Exchange | 最终失败事件 |
| `im.message.fanout.v1` | Durable Classic Queue | 消费消息创建事件 |
| `im.message.retry.5s.v1` | TTL Queue | 5 秒后重试 |
| `im.message.retry.30s.v1` | TTL Queue | 30 秒后重试 |
| `im.message.retry.2m.v1` | TTL Queue | 2 分钟后重试 |
| `im.message.dlq.v1` | Dead Letter Queue | 达到重试上限或不可重试事件 |

应用启动时会幂等声明拓扑。本地演示使用 Classic Queue；生产环境具备多节点 Broker 后，再评估 Quorum Queue。

## 3. 发布语义

- Outbox 使用 `FOR UPDATE SKIP LOCKED` 领取事件，多实例不会同时持有同一条记录。
- 领取后写入实例 ID 和处理租约，避免进程崩溃后事件永久停在“处理中”。
- RabbitMQ 消息使用 `DeliveryMode=Persistent`、`mandatory=true` 和 Publisher Confirm。
- Confirm 成功后才会把 Outbox 标记为已发布。
- Broker 失败、Confirm 超时或不可路由时，事件按指数退避回到待发布状态。
- 达到最大尝试次数后进入 Outbox 死信状态，保留 `last_error` 供排查。
- Confirm 成功但数据库状态更新失败时可能重复发布，因此消费者必须幂等。

## 4. 消费语义

- Consumer 使用独立连接和 Channel，设置 Prefetch，关闭 Auto ACK。
- 每 10 秒为一次业务处理设置超时。
- 成功处理后手动 ACK。
- 可重试错误依次进入 5 秒、30 秒和 2 分钟 TTL 队列。
- 三次重试后重新投递回主队列时进入 DLQ。
- JSON、字段、版本或消息事实错误属于不可重试错误，直接 NACK 进入 DLQ。
- `processed_events(event_id, consumer_name)` 保证同一消费者重复处理时不会重复执行业务状态。

当前 Consumer 已完成事件校验和消费幂等登记。接入新的单写 WebSocket 连接层后，会在同一处理边界增加在线用户扇出；当前不会通过旧的并发不安全连接管理器直接写 WebSocket。

## 5. 本地配置

在 `internal/config/app.env` 中增加：

```text
GINCHAT_RABBITMQ_URL=amqp://用户名:密码@127.0.0.1:5672/ginchat
```

URL 必须与应用使用的 RabbitMQ VHost 一致。`deploy/.env` 只用于 Compose 容器，不会被 Go 应用自动加载。

启动应用：

```powershell
.\scripts\run-server.ps1
```

观测队列：

```powershell
docker exec ginchat-rabbitmq rabbitmqctl list_queues name messages messages_ready messages_unacknowledged
```

管理界面：

```text
http://localhost:15672
```
