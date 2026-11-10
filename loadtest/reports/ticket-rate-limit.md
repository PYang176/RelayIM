# WebSocket Ticket 限流验证

## 1. 测试目标

验证瞬时 Ticket 洪峰下：

- Redis 全局与单用户滑动窗口限流是否原子生效。
- 超额请求是否返回 `429 Too Many Requests`。
- 响应是否包含 `Retry-After` 和限流范围。
- 等待窗口后请求是否恢复。

## 2. 配置

| 配置 | 值 |
| --- | ---: |
| 全局窗口上限 | 250 次 |
| 单用户窗口上限 | 10 次 |
| 窗口长度 | 1,000 ms |
| 同时请求数 | 同一用户 20 次 |

滑动窗口只保存最近一秒内已经放行的请求，放行时才写入 ZSET；被拒绝请求不会继续增加窗口计数。

## 3. 结果

| 指标 | 结果 |
| --- | ---: |
| 成功申请 Ticket | 10 |
| 返回 429 | 10 |
| 未知错误 | 0 |
| 等待 1.2 秒后重新申请 | 成功 |
| 检查通过率 | 100% |

限流拒绝会记录：

```text
relayim_rate_limit_rejected_total{scope="user"}
```

## 4. 原始摘要

[ticket-rate-limit.json](./ticket-rate-limit.json)

## 5. 5,000 瞬时 TCP 洪峰边界

在 Docker Desktop 中使用 `host.docker.internal` 向 Windows 原生 Go 服务瞬时发起 5,000 次连接时，部分请求在进入 Gin HTTP Handler 前就被 Docker 端口代理拒绝，表现为：

```text
dial tcp 192.168.65.254:8081: connect: connection refused
```

这类失败不是应用返回的 429，应用层限流无法处理尚未建立的 TCP 连接。后续需要：

- 使用 Windows 原生 k6，消除 Docker NAT 变量。
- 或将 Go 服务也放入 Docker Compose，在同一容器网络中测试。
- 生产环境应由负载均衡器和连接队列处理 TCP 洪峰，应用限流负责已经进入 HTTP 层的超额请求。

## 6. 复现

```powershell
docker run --rm `
  --add-host=host.docker.internal:host-gateway `
  -e BASE_URL=http://host.docker.internal:8081 `
  -e SUMMARY_FILE=ticket-rate-limit.json `
  -v "${PWD}:/work" `
  -w /work `
  grafana/k6:0.57.0 run loadtest/k6/ticket-rate-limit.js
```
