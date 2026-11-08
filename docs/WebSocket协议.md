# WebSocket 协议

## 1. 建连

客户端先调用受保护接口：

```http
POST /api/v1/auth/ws-ticket
Authorization: Bearer <JWT>
```

响应：

```json
{
  "ticket": "短时随机票据",
  "expires_in_seconds": 30,
  "device_id": "1839281749283749120"
}
```

再使用一次性 Ticket 建连：

```text
ws://127.0.0.1:8081/ws?ticket=<一次性票据>
```

Ticket 有效期 30 秒，并在 Redis 中使用 `GETDEL` 消费，因此重放同一 Ticket 会失败。`/ws` 路径不写 Gin 访问日志。

Ticket 只用于建立连接，不替代 Access Token。Access Token 继续用于 HTTP 接口，新的 Ticket 必须通过 HTTP 认证后重新申请。

## 2. 连接模型

- 同一用户可以同时拥有多个连接，最多 10 个。
- 每个连接有独立的 64 容量有界发送队列。
- 每个连接只有一个写协程，其他业务协程只能调用 `Enqueue`。
- 队列满时不阻塞 HTTP 或 RabbitMQ Consumer，关闭慢连接并等待客户端重连补拉。
- 服务端每 25 秒发送 Ping，60 秒未收到 Pong 时关闭连接。
- 客户端单帧最大 64 KB。
- 连接按具体对象移除，旧连接退出不会误删同一用户的新连接。

## 3. 在线消息

RabbitMQ Consumer 完成事件幂等校验后，会向同一会话全部有效成员提交。发送者账号的其他设备也会收到事件，客户端按 `message_id` 去重：

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

实时消息可能因重试、跨实例节点队列或网络延迟乱序到达。客户端维护每个会话的连续 `seq` 水位：

- 收到大于 `contiguous_seq + 1` 的消息时记录缺口，并立即使用 HTTP `after_seq` 补拉。
- 只有连续无缺口时才推进 `last_read_seq`，不能把当前最大已见序号直接作为已读游标。
- 晚到消息按 `message_id` 去重后重新按 `seq` 排序。
- 补拉失败按退避重试，断线重连仍从连续水位恢复。

图片或文件消息的 `msg_type` 为 `2` 或 `3`，并增加受保护的内容路径和原始文件名：

```json
{
  "action": "message",
  "message_id": "1839281749283749121",
  "conversation_id": "1839281749283749120",
  "sender_id": "1839281749283749001",
  "seq": 7,
  "msg_type": 2,
  "content": "",
  "url": "/api/v1/attachments/1839281749283749122/content",
  "file_name": "截图.png"
}
```

客户端收到该事件后调用 `GET /api/v1/attachments/:id` 换取新的短时预签名地址。WebSocket 不广播永久对象地址，预签名 URL 也不能写入数据库。

连接建立和每次 Pong 会刷新 Redis Presence 中当前设备的活跃时间。连接正常关闭只删除当前设备，不影响同一用户其他设备。在线状态查询见 [在线状态](./在线状态.md)。

## 4. 在线撤回

消息撤回事务提交后，服务端向会话内全部在线成员广播：

```json
{
  "action": "message_recalled",
  "message_id": "1839281749283749121",
  "conversation_id": "1839281749283749120",
  "sender_id": "1839281749283749001",
  "recalled_by": "1839281749283749002",
  "seq": 7,
  "recalled_at": "2026-11-07T08:31:00Z"
}
```

客户端收到后按 `message_id` 原地替换消息：清空正文和附件字段，显示“该消息已撤回”。如果事件乱序或重连，历史接口中的 `recalled=true` 是最终事实来源。

## 5. 客户端回执

收到消息后，客户端按设备发送送达回执：

```json
{
  "action": "ack",
  "message_id": "1839281749283749121"
}
```

服务端使用 Ticket 绑定的设备 ID 写入 `message_deliveries(message_id, device_id)`，重复 ACK 幂等。

用户阅读到某个位置后发送：

```json
{
  "action": "read",
  "conversation_id": "1839281749283749120",
  "seq": 11
}
```

服务端单调推进 `conversation_members.last_read_seq`，较小序号不会覆盖较大序号。

## 6. Origin 与安全边界

- 允许同 Host Origin。
- 允许 `localhost` 和回环 IP，支持本地前端开发。
- 其他第三方 Origin 默认拒绝。
- 服务端不打印 Token、完整消息正文或密码。

## 7. 当前限制

每个进程内仍使用独立连接注册表，但跨实例投递已经通过 Redis 设备路由、`im.delivery` Direct Exchange 和 `im.delivery.node.<instance_id>` 临时队列完成，详见 [多实例实时投递](./多实例实时投递.md)。

节点队列是临时队列。实例退出或实时命令不可路由时，在线帧允许丢失，客户端重连后使用 HTTP `after_seq` 恢复。

连接断开后，客户端调用：

```http
GET /api/v1/conversations/:conversation_id/messages?after_seq=<最后已见seq>
```

按 `seq` 升序补拉断线期间消息，避免依赖 WebSocket 重放。实时连接和 HTTP 补拉可能重复，客户端必须按 `message_id` 去重。
