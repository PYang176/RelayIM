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
  "expires_in_seconds": 30
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

送达和已读回执暂未启用。旧 `message` 表不在当前数据库迁移中，因此 WebSocket Handler 不再调用旧回执服务。

下一阶段需要基于以下新版结构实现：

- `message_deliveries`：按消息和设备记录送达。
- `conversation_members.last_read_seq`：按用户维护已读游标。

## 5. Origin 与安全边界

- 允许同 Host Origin。
- 允许 `localhost` 和回环 IP，支持本地前端开发。
- 其他第三方 Origin 默认拒绝。
- 服务端不打印 Token、完整消息正文或密码。

## 6. 当前限制

连接注册表仅存在单个 Go 进程内。多实例部署时，消费者需要先通过 Redis 查询用户连接所在实例，再经 `im.delivery.node.<instance_id>` 队列转发投递命令。当前阶段只支持单实例实时扇出。

连接断开后，客户端调用：

```http
GET /api/v1/conversations/:conversation_id/messages?after_seq=<最后已见seq>
```

按 `seq` 升序补拉断线期间消息，避免依赖 WebSocket 重放。实时连接和 HTTP 补拉可能重复，客户端必须按 `message_id` 去重。
