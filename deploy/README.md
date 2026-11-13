# RelayIM Docker 部署

Compose 支持两种启动方式：

- 只启动基础设施，Go 和 Vue 继续使用本机开发服务器。
- 启动容器化应用，由 Docker 构建 Go 后端和 Nginx 前端，MySQL 始终使用宿主机服务。

基础设施包括：

- RabbitMQ 4.3.7，包含管理界面和 Prometheus 指标插件。
- Redis 7.4，启用 AOF 持久化和密码认证。
- SeaweedFS 4.52，用于图片和文件的 S3 兼容对象存储。
- Tempo 2.7，用于接收 OpenTelemetry OTLP Trace。
- Prometheus，负责采集指标。
- Grafana，负责展示指标面板。

两套 Compose 都不包含 MySQL。业务数据由宿主机 MySQL 8 保存，应用和迁移容器通过 `host.docker.internal:3306` 访问。

`deploy/.env` 保存 Compose 基础设施变量，不会自动注入 Windows 上直接运行的 Go 应用。
应用配置及其 `RELAYIM_*` 环境变量说明见
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

## 容器化应用启动

启动前确认本机 MySQL 已运行，并创建数据库：

```sql
CREATE DATABASE relayim
  CHARACTER SET utf8mb4
  COLLATE utf8mb4_0900_ai_ci;
```

首次运行时创建应用容器变量文件：

```powershell
Copy-Item deploy\app.env.example deploy\app.env
```

填写 `deploy\.env` 和 `deploy\app.env` 中的本地 MySQL 地址、密码、JWT 密钥。默认配置是：

```text
RELAYIM_MYSQL_HOST=host.docker.internal
RELAYIM_MYSQL_PORT=3306
RELAYIM_MYSQL_USER=root
RELAYIM_MYSQL_PASSWORD=<本机 MySQL 密码>
RELAYIM_MYSQL_DBNAME=relayim
```

已有 `deploy\.env` 如果仍使用 `RELAYIM_MYSQL_ROOT_PASSWORD`，应改为 `RELAYIM_MYSQL_PASSWORD`，并确认值来自本机 MySQL，而不是旧容器 MySQL。

然后在项目根目录执行：

```powershell
docker compose `
  --env-file deploy\.env `
  --env-file deploy\app.env `
  -f deploy\docker-compose.yml `
  -f deploy\docker-compose.app.yml `
  up -d --build --remove-orphans
```

`--remove-orphans` 会清掉旧 Compose 文件创建的 `relayim-mysql` 容器。旧 `mysql_data` 数据卷不会被自动删除，确认不再需要旧容器数据后再手动清理。

启动顺序如下：

1. 本机 MySQL 已启动，Redis、RabbitMQ 通过容器健康检查。
2. 一次性 `migrate` 容器连接本机 MySQL 并执行 `goose up`。
3. Go 后端启动并通过健康检查。
4. Nginx 提供前端静态文件、API 和 WebSocket 反向代理。

访问地址：

| 服务 | 地址 |
| --- | --- |
| RelayIM Web | http://localhost:8080 |
| Go API | http://localhost:8081 |
| 本机 MySQL | `127.0.0.1:3306` |

构建产物使用多阶段镜像：

- 根目录 `Dockerfile` 的 `runtime` target 构建静态 Go 二进制，以非 root 用户运行。
- 根目录 `Dockerfile` 的 `migration` target 提供 Goose 和 migration 文件。
- `web/Dockerfile` 构建 Vue 静态资源并复制到 Nginx。

`app.env` 中的 `RELAYIM_S3_PUBLIC_ENDPOINT` 控制浏览器看到的附件预签名地址。默认值 `http://localhost:8333` 直接指向宿主机映射的 SeaweedFS 端口；外部部署时应改成浏览器实际可访问的对象存储域名。

查看完整栈状态和日志：

```powershell
docker compose `
  --env-file deploy\.env `
  --env-file deploy\app.env `
  -f deploy\docker-compose.yml `
  -f deploy\docker-compose.app.yml `
  ps

docker compose `
  --env-file deploy\.env `
  --env-file deploy\app.env `
  -f deploy\docker-compose.yml `
  -f deploy\docker-compose.app.yml `
  logs -f app web
```

停止容器化应用并保留 RabbitMQ、Redis 等数据卷，本机 MySQL 不受影响：

```powershell
docker compose `
  --env-file deploy\.env `
  --env-file deploy\app.env `
  -f deploy\docker-compose.yml `
  -f deploy\docker-compose.app.yml `
  down
```

只有明确接受 RabbitMQ、对象存储和监控数据丢失时，才追加 `-v` 删除数据卷。本机 MySQL 数据目录不受 Compose 卷管理。

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
| Redis | `localhost:6380` |
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
relayim
```

此时 Windows 上的 Go 程序使用以下连接地址：

```text
amqp://<RABBITMQ_DEFAULT_USER>:<RABBITMQ_DEFAULT_PASS>@localhost:5672/relayim
```

配置示例：

```text
amqp://admin:你的密码@localhost:5672/relayim
```

### 已有数据卷的 VHost 初始化

修改 `.env` 中的 `RABBITMQ_VHOST` 不会自动修改已经存在的 RabbitMQ 数据卷。

如果当前 VHost 是 `/relayim`，需要一次性创建新的 `relayim` VHost：

```powershell
docker exec relayim-rabbitmq rabbitmqctl add_vhost relayim
docker exec relayim-rabbitmq rabbitmqctl set_permissions -p relayim admin ".*" ".*" ".*"
```

随后修改 `deploy\.env`：

```text
RABBITMQ_VHOST=relayim
```

确认 VHost：

```powershell
docker exec relayim-rabbitmq rabbitmqctl list_vhosts
```

也可以保留已有的 `/relayim` VHost，但连接地址必须对前导斜杠编码：

```text
amqp://admin:你的密码@localhost:5672/%2Frelayim
```

## 指标状态

RabbitMQ 的 Prometheus 指标地址是：

```text
rabbitmq:15692
```

`relayim-app` 的抓取地址是：

```text
host.docker.internal:8081
```

Go 服务提供 `/metrics`、`/health/live` 和 `/health/ready`。Grafana 会自动加载 `RelayIM Overview` 面板，详细说明见
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
relayim-media
```

可以通过 `S3_BUCKET` 修改。

MySQL 只保存附件的对象键、MIME 类型、大小和校验值等元数据，图片和文件本体保存在 SeaweedFS 中。
