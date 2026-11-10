# GinChat Docker 基础设施

这套 Compose 配置用于启动项目依赖，暂时不要求把 Go 应用也放进容器：

- RabbitMQ 4.3.7，包含管理界面和 Prometheus 指标插件。
- Redis 7.4，启用 AOF 持久化和密码认证。
- SeaweedFS 4.52，用于图片和文件的 S3 兼容对象存储。
- Tempo 2.7，用于接收 OpenTelemetry OTLP Trace。
- Prometheus，负责采集指标。
- Grafana，负责展示指标面板。

MySQL 暂不放入 Compose。开发环境继续使用 Windows 本机的 MySQL 8.0.34。

`deploy/.env` 只保存 Compose 基础设施变量，不会自动注入 Windows 上直接运行的 Go 应用。
应用配置及其 `GINCHAT_*` 环境变量说明见
[配置管理指南](../docs/配置管理指南.md)。

## 首次启动

在项目根目录执行：

```powershell
Copy-Item deploy\.env.example deploy\.env
```

修改 `deploy\.env`，把示例密码替换为本地开发密码。

启动全部基础设施：

```powershell
docker compose --env-file deploy\.env -f deploy\docker-compose.yml up -d
```

默认会启动 RabbitMQ、Redis、SeaweedFS、Prometheus 和 Grafana。

查看状态和日志：

```powershell
docker compose --env-file deploy\.env -f deploy\docker-compose.yml ps
docker compose --env-file deploy\.env -f deploy\docker-compose.yml logs -f rabbitmq
```

停止服务但保留数据：

```powershell
docker compose --env-file deploy\.env -f deploy\docker-compose.yml down
```

删除容器和全部持久化数据：

```powershell
docker compose --env-file deploy\.env -f deploy\docker-compose.yml down -v
```

只有确认 RabbitMQ 消息、SeaweedFS 文件、Prometheus 指标和 Grafana 配置都可以丢弃时，才能使用 `down -v`。

## 本地访问地址

| 服务 | 地址 |
| --- | --- |
| RabbitMQ AMQP | `localhost:5672` |
| RabbitMQ 管理界面 | http://localhost:15672 |
| Redis | `localhost:6379` |
| SeaweedFS S3 | http://localhost:8333 |
| Prometheus | http://localhost:9090 |
| Grafana | http://localhost:3000 |
| Tempo OTLP HTTP | http://localhost:4318 |
| Tempo HTTP API | http://localhost:3200 |

账号和密码以 `deploy\.env` 为准。

## RabbitMQ VHost

AMQP URI 中的 `/` 是路径分隔符，不是 VHost 名称的一部分。

推荐使用不带前导斜杠的 VHost 名称：

```text
ginchat
```

此时 Windows 上的 Go 程序使用以下连接地址：

```text
amqp://<RABBITMQ_DEFAULT_USER>:<RABBITMQ_DEFAULT_PASS>@localhost:5672/ginchat
```

配置示例：

```text
amqp://admin:你的密码@localhost:5672/ginchat
```

### 已有数据卷的 VHost 初始化

修改 `.env` 中的 `RABBITMQ_VHOST` 不会自动修改已经存在的 RabbitMQ 数据卷。

如果当前 VHost 是 `/ginchat`，需要一次性创建新的 `ginchat` VHost：

```powershell
docker exec ginchat-rabbitmq rabbitmqctl add_vhost ginchat
docker exec ginchat-rabbitmq rabbitmqctl set_permissions -p ginchat admin ".*" ".*" ".*"
```

随后修改 `deploy\.env`：

```text
RABBITMQ_VHOST=ginchat
```

确认 VHost：

```powershell
docker exec ginchat-rabbitmq rabbitmqctl list_vhosts
```

也可以保留已有的 `/ginchat` VHost，但连接地址必须对前导斜杠编码：

```text
amqp://admin:你的密码@localhost:5672/%2Fginchat
```

## 指标状态

RabbitMQ 的 Prometheus 指标地址是：

```text
rabbitmq:15692
```

`ginchat-app` 的抓取地址是：

```text
host.docker.internal:8081
```

Go 服务提供 `/metrics`、`/health/live` 和 `/health/ready`。Grafana 会自动加载 `GinChat Overview` 面板，详细说明见
[可观测性](../docs/可观测性.md)。

## 对象存储

SeaweedFS 的 S3 兼容接口地址是：

```text
http://localhost:8333
```

当前 Compose 使用以下变量作为 S3 凭据：

```text
MINIO_ROOT_USER
MINIO_ROOT_PASSWORD
```

这样命名是为了兼容当前环境文件。后续实现媒体功能时可以统一改成更中性的 `S3_ACCESS_KEY_ID` 和 `S3_SECRET_ACCESS_KEY`。

默认 Bucket：

```text
ginchat-media
```

可以通过 `S3_BUCKET` 修改。

MySQL 只保存附件的对象键、MIME 类型、大小和校验值等元数据，图片和文件本体保存在 SeaweedFS 中。
