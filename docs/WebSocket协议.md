# WebSocket 协议

## 1. 建连

当前过渡实现使用查询参数：

```text
ws://127.0.0.1:8081/ws?token=<JWT>
```

服务端先校验 JWT、Redis 黑名单和白名单，再升级连接。`/ws` 路径不写 Gin 访问日志，避免完整 Token 落盘。

查询参数 Token 仍可能进入浏览器历史或代理日志。正式前端阶段应改为短时、单次有效的 `ws-ticket`；Ticket 只用于建立连接，不替代 Access Token。

## 2. 连接模型

- 同一用户可以同时拥有多个连接，最多 10 个。
- 每个连接有独立的 64 容量有界发送队列。
- 每个连接只有一个写协程，其他业务协程只能调用 `Enqueue`。
- 队列满时不阻塞 HTTP 或 RabbitMQ Consumer，关闭慢连接并等待客户端重连补拉。
- 服务端每 25 秒发送 Ping，60 秒未收到 Pong 时关闭连接。
- 客户端单帧最大 64 KB。
- 连接按具体对象移除，旧连接退出不会误删同一用户的新连接。

## 3. 在线消息

RabbitMQ Consumer 完成事件幂等校验后，会向同一会话中的其他在线成员提交：

```json
{
  "action": "message",
  "message_id": "1839281749283749121",
  "conversation_id": "1839281749283749120",
  "sender_id": "1839281749283749001",
  "seq": 7,
  "msg_type": 1,
  "content": "你好"
}
```

客户端必须以 `message_id` 去重，并按 `conversation_id + seq` 排序。RabbitMQ 采用至少一次投递，不能依赖 WebSocket 帧顺序作为最终消息顺序。

## 4. 客户端回执

送达：

```json
{
  "action": "ack",
  "message_id": "1839281749283749121"
}
```

已读：

```json
{
  "action": "read",
  "message_id": "1839281749283749121"
}
```

新版消息已实现 `message_deliveries` 与 `conversation_members.last_read_seq` 数据结构，但回执接口仍需迁移到新版会话模型。

## 5. Origin 与安全边界

- 允许同 Host Origin。
- 允许 `localhost` 和回环 IP，支持本地前端开发。
- 其他第三方 Origin 默认拒绝。
- 服务端不打印 Token、完整消息正文或密码。

## 6. 当前限制

连接注册表仅存在单个 Go 进程内。多实例部署时，消费者需要先通过 Redis 查询用户连接所在实例，再经 `im.delivery.node.<instance_id>` 队列转发投递命令。当前阶段只支持单实例实时扇出。

历史消息和断线补拉尚未迁移到新版 `messages` 表，因此 WebSocket 建连时不再调用旧 `message` 表。下一阶段需要通过新版 HTTP 历史接口按 `after_seq` 补拉。
