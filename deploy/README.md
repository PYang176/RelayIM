# GinChat Docker Infrastructure

This Compose stack starts the project dependencies that do not require the Go
application to be containerized yet:

- RabbitMQ 4.3.7 with management and Prometheus plugins.
- Redis 7.4 with AOF persistence and password authentication.
- A legacy MinIO image behind an optional profile. MinIO Community Edition is
  source-only and no longer maintained, so object storage will be selected
  during the media milestone.
- Prometheus for metrics collection.
- Grafana for dashboards.

MySQL is intentionally not included. The development environment continues to
use the existing MySQL 8.0.34 service on the Windows host.

## First start

Run these commands from the repository root:

```powershell
Copy-Item deploy\.env.example deploy\.env
```

Edit `deploy\.env` and replace every password.

Start the stack:

```powershell
docker compose --env-file deploy\.env -f deploy\docker-compose.yml up -d
```

The default stack starts RabbitMQ, Redis, Prometheus and Grafana. The legacy
MinIO container is excluded unless its `legacy-minio` profile is enabled.

Check status and logs:

```powershell
docker compose --env-file deploy\.env -f deploy\docker-compose.yml ps
docker compose --env-file deploy\.env -f deploy\docker-compose.yml logs -f rabbitmq
```

Stop the stack without deleting data:

```powershell
docker compose --env-file deploy\.env -f deploy\docker-compose.yml down
```

To remove containers and persisted data:

```powershell
docker compose --env-file deploy\.env -f deploy\docker-compose.yml down -v
```

Do not use `down -v` unless RabbitMQ messages, MinIO files, Prometheus data and
Grafana configuration may be discarded.

## Local endpoints

| Service | URL or address |
| --- | --- |
| RabbitMQ AMQP | `localhost:5672` |
| RabbitMQ Management | http://localhost:15672 |
| Redis | `localhost:6379` |
| Prometheus | http://localhost:9090 |
| Grafana | http://localhost:3000 |

Use the credentials configured in `deploy\.env`.

The RabbitMQ connection URL for a Go process running on Windows is:

```text
amqp://<RABBITMQ_DEFAULT_USER>:<RABBITMQ_DEFAULT_PASS>@localhost:5672/<RABBITMQ_VHOST>
```

The vhost path must be URL-safe. For the example vhost `/ginchat`, the URL is:

```text
amqp://ginchat:change-me@localhost:5672/ginchat
```

## Metrics status

RabbitMQ metrics are available to Prometheus at `rabbitmq:15692`.

The `ginchat-app` scrape target points to `host.docker.internal:8081`. It will
remain down until the Go service implements a Prometheus `/metrics` endpoint.
This is expected during the initial setup.

## Legacy MinIO profile

MinIO Community Edition is no longer distributed as a maintained container
image. The Compose file keeps the old image only behind an optional profile so
that it cannot block the default development environment.

Do not enable this profile unless the legacy image has been reviewed and is
available through a trusted registry:

```powershell
docker compose --env-file deploy\.env -f deploy\docker-compose.yml --profile legacy-minio up -d minio
```

The media storage decision must be revisited before implementing image and file
messages.
