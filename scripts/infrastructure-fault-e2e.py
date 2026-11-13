"""验证 Docker 中 RabbitMQ 和 Redis 短暂故障后的服务恢复行为。"""

from __future__ import annotations

import argparse
import json
import os
import re
import subprocess
import time
from datetime import UTC, datetime
from pathlib import Path
from typing import Any, Callable

from playwright.sync_api import APIRequestContext, sync_playwright

PASSWORD = "Fault-test-123456!"


def api_request(
    api: APIRequestContext,
    method: str,
    path: str,
    token: str = "",
    payload: dict[str, Any] | None = None,
) -> tuple[int, Any]:
    """发送 API 请求并返回状态码和解析后的响应。"""
    headers = {}
    data = None
    if payload is not None:
        headers["Content-Type"] = "application/json"
        data = json.dumps(payload, ensure_ascii=False)
    if token:
        headers["Authorization"] = f"Bearer {token}"
    try:
        response = api.fetch(
            f"/api/v1{path}",
            method=method,
            headers=headers,
            data=data,
            timeout=15_000,
        )
    except Exception as exc:
        return 0, {"error": str(exc)}
    content = response.text()
    return response.status, json.loads(content) if content else None


def api_ok(
    api: APIRequestContext,
    method: str,
    path: str,
    token: str = "",
    payload: dict[str, Any] | None = None,
) -> Any:
    """请求接口并要求返回 2xx，否则抛出带响应正文的异常。"""
    status, body = api_request(api, method, path, token, payload)
    if status < 200 or status >= 300:
        raise RuntimeError(f"{method} {path} 返回 {status}: {body}")
    return body


def create_user(
    api: APIRequestContext,
    username: str,
    nickname: str,
) -> dict[str, Any]:
    """注册并登录一个故障注入测试账号。"""
    user = api_ok(
        api,
        "POST",
        "/auth/register",
        payload={
            "username": username,
            "password": PASSWORD,
            "confirm_password": PASSWORD,
            "nickname": nickname,
        },
    )
    login = login_user(api, username)
    return {
        "id": str(user["id"]),
        "username": username,
        "nickname": nickname,
        "token": login["token"],
    }


def login_user(api: APIRequestContext, username: str) -> dict[str, Any]:
    """登录故障注入测试账号并返回会话信息。"""
    return api_ok(
        api,
        "POST",
        "/auth/login",
        payload={
            "username": username,
            "password": PASSWORD,
            "device_name": "基础设施故障注入",
            "platform": 4,
        },
    )


def prepare_conversation(
    api: APIRequestContext,
    sender: dict[str, Any],
    receiver: dict[str, Any],
) -> str:
    """创建好友关系和私聊会话。"""
    api_ok(
        api,
        "POST",
        "/friend-requests",
        token=sender["token"],
        payload={
            "target_user_id": receiver["id"],
            "remark": "基础设施故障注入测试",
        },
    )
    received = api_ok(
        api,
        "GET",
        "/friend-requests/received",
        token=receiver["token"],
    )
    request = next(
        item
        for item in received["requests"]
        if str(item["user"]["id"]) == sender["id"]
    )
    api_ok(
        api,
        "POST",
        f"/friend-requests/{request['id']}/accept",
        token=receiver["token"],
    )
    conversation = api_ok(
        api,
        "POST",
        "/conversations/private",
        token=sender["token"],
        payload={"peer_user_id": receiver["id"]},
    )
    return conversation["id"]


def send_message(
    api: APIRequestContext,
    token: str,
    conversation_id: str,
    client_message_id: str,
    content: str,
) -> tuple[int, Any]:
    """通过消息接口发送一条文本消息。"""
    return api_request(
        api,
        "POST",
        f"/conversations/{conversation_id}/messages",
        token=token,
        payload={
            "client_message_id": client_message_id,
            "content": content,
        },
    )


def docker_container(container: str, action: str) -> None:
    """启动或停止指定 Docker 容器。"""
    subprocess.run(
        ["docker", action, container],
        check=True,
        capture_output=True,
        text=True,
        timeout=90,
    )


def container_health(container: str) -> str:
    """读取 Docker 容器的运行和健康状态。"""
    result = subprocess.run(
        [
            "docker",
            "inspect",
            "--format",
            '{{.State.Status}}|{{if .State.Health}}{{.State.Health.Status}}{{else}}none{{end}}',
            container,
        ],
        check=True,
        capture_output=True,
        text=True,
        timeout=15,
    )
    state, health = result.stdout.strip().split("|", 1)
    return health if state == "running" and health != "none" else state


def wait_until(
    check: Callable[[], bool],
    timeout_seconds: float,
    interval_seconds: float = 0.5,
) -> float:
    """轮询条件并在超时后抛出异常，返回等待耗时。"""
    started = time.monotonic()
    deadline = started + timeout_seconds
    while time.monotonic() < deadline:
        if check():
            return round((time.monotonic() - started) * 1000, 2)
        time.sleep(interval_seconds)
    raise TimeoutError(f"等待条件超过 {timeout_seconds:g} 秒")


def wait_container_healthy(container: str, timeout_seconds: float = 120) -> float:
    """等待容器达到 healthy 状态。"""
    return wait_until(
        lambda: container_health(container) == "healthy",
        timeout_seconds,
        1,
    )


def http_status(url: str) -> int:
    """读取 HTTP 接口状态码；连接失败时返回零。"""
    import urllib.error
    import urllib.request

    try:
        with urllib.request.urlopen(url, timeout=5) as response:
            return response.status
    except urllib.error.HTTPError as exc:
        return exc.code
    except Exception:
        return 0


def metric_sum(metrics: str, pattern: str) -> float:
    """按正则累加 Prometheus 指标值。"""
    return sum(float(value) for value in re.findall(pattern, metrics))


def read_metrics(base_url: str) -> str:
    """读取应用 Prometheus 指标文本。"""
    return api_text(f"{base_url}/metrics")


def api_text(url: str) -> str:
    """读取普通文本 HTTP 响应。"""
    import urllib.request

    with urllib.request.urlopen(url, timeout=5) as response:
        return response.read().decode("utf-8")


def outbox_status(metrics: str, status: str) -> float:
    """读取指定状态的 Outbox 事件数量。"""
    match = re.search(
        rf'^relayim_outbox_events\{{status="{status}"\}}\s+([0-9.eE+\-]+)',
        metrics,
        re.MULTILINE,
    )
    return float(match.group(1)) if match else 0


def rabbit_queue_counts(container: str, vhost: str = "relayim") -> dict[str, dict[str, int]]:
    """读取 RabbitMQ 队列消息数和消费者数量。"""
    result = subprocess.run(
        [
            "docker",
            "exec",
            container,
            "rabbitmqctl",
            "-q",
            "-p",
            vhost,
            "list_queues",
            "name",
            "messages",
            "consumers",
        ],
        check=True,
        capture_output=True,
        text=True,
        timeout=90,
    )
    queues: dict[str, dict[str, int]] = {}
    for line in result.stdout.splitlines():
        parts = line.split("\t")
        if len(parts) != 3 or not parts[0].startswith("im."):
            continue
        queues[parts[0]] = {
            "messages": int(parts[1]),
            "consumers": int(parts[2]),
        }
    return queues


def rabbitmq_fault(
    api: APIRequestContext,
    sender: dict[str, Any],
    conversation_id: str,
    base_url: str,
    container: str,
) -> dict[str, Any]:
    """验证 RabbitMQ 中断期间消息仍可入库并在恢复后完成消费。"""
    before_metrics = read_metrics(base_url)
    queues_before = rabbit_queue_counts(container)
    dlq_before = queues_before.get("im.events.dlq.v2", {}).get("messages", 0)
    consumed_before = metric_sum(
        before_metrics,
        r'^relayim_message_consumer_events_total\{event_type="message\.created\.v1",result="success"\}\s+([0-9.eE+\-]+)',
    )
    docker_container(container, "stop")
    wait_until(lambda: container_health(container) == "exited", 30)
    time.sleep(2)

    statuses = []
    for index in range(3):
        status, body = send_message(
            api,
            sender["token"],
            conversation_id,
            f"rabbit-down-{time.time_ns()}-{index}",
            f"RabbitMQ 中断期间消息 {index + 1}",
        )
        statuses.append(status)
        if status != 201:
            raise AssertionError(f"RabbitMQ 中断期间消息应写入 Outbox: {status} {body}")

    max_pending = 0
    deadline = time.monotonic() + 15
    while time.monotonic() < deadline:
        metrics = read_metrics(base_url)
        max_pending = max(max_pending, outbox_status(metrics, "pending"))
        time.sleep(1)

    recovery_started = time.monotonic()
    docker_container(container, "start")
    wait_container_healthy(container)
    def rabbit_recovered() -> bool:
        """判断 Outbox、主消费队列和重试队列是否全部排空。"""
        metrics = read_metrics(base_url)
        queues = rabbit_queue_counts(container)
        main_queue = queues.get("im.events.fanout.v2", {})
        retry_messages = sum(
            value["messages"]
            for name, value in queues.items()
            if name.startswith("im.events.retry.")
        )
        dlq_messages = queues.get("im.events.dlq.v2", {}).get("messages", 0)
        return (
            outbox_status(metrics, "pending") == 0
            and outbox_status(metrics, "processing") == 0
            and outbox_status(metrics, "dead") == 0
            and main_queue.get("messages", 0) == 0
            and main_queue.get("consumers", 0) >= 1
            and retry_messages == 0
            and dlq_messages <= dlq_before
        )

    wait_until(rabbit_recovered, 240, 2)
    recovery_ms = round((time.monotonic() - recovery_started) * 1000, 2)
    messages = api_ok(
        api,
        "GET",
        f"/conversations/{conversation_id}/messages?limit=100",
        token=sender["token"],
    )
    contents = {item["content"] for item in messages["messages"]}
    recovered = all(f"RabbitMQ 中断期间消息 {index + 1}" in contents for index in range(3))
    if not recovered:
        raise AssertionError("RabbitMQ 恢复后缺少中断期间消息")
    queues_after = rabbit_queue_counts(container)
    consumed_after = metric_sum(
        read_metrics(base_url),
        r'^relayim_message_consumer_events_total\{event_type="message\.created\.v1",result="success"\}\s+([0-9.eE+\-]+)',
    )
    return {
        "message_statuses_during_outage": statuses,
        "max_outbox_pending": max_pending,
        "recovery_ms": recovery_ms,
        "messages_recovered": recovered,
        "consumer_success_delta": consumed_after - consumed_before,
        "dlq_messages_before": dlq_before,
        "dlq_messages_after": queues_after.get("im.events.dlq.v2", {}).get(
            "messages",
            0,
        ),
        "main_queue": queues_after.get("im.events.fanout.v2", {}),
    }


def redis_fault(
    api: APIRequestContext,
    sender: dict[str, Any],
    conversation_id: str,
    base_url: str,
    container: str,
) -> dict[str, Any]:
    """验证 Redis 中断时的认证失败和恢复后的重新登录。"""
    docker_container(container, "stop")
    wait_until(lambda: container_health(container) == "exited", 30)
    time.sleep(1)
    auth_status, _ = api_request(api, "GET", "/me", token=sender["token"])
    send_status, _ = send_message(
        api,
        sender["token"],
        conversation_id,
        f"redis-down-{time.time_ns()}",
        "Redis 中断期间消息",
    )
    ready_status = http_status(f"{base_url}/health/ready")
    if auth_status != 503 or send_status != 503 or ready_status != 503:
        raise AssertionError(
            f"Redis 中断状态不符合预期: auth={auth_status} send={send_status} ready={ready_status}"
        )

    recovery_started = time.monotonic()
    docker_container(container, "start")
    wait_container_healthy(container)
    wait_until(lambda: http_status(f"{base_url}/health/ready") == 200, 60, 1)
    login = login_user(api, sender["username"])
    sender["token"] = login["token"]
    send_after, body = send_message(
        api,
        sender["token"],
        conversation_id,
        f"redis-recovered-{time.time_ns()}",
        "Redis 恢复后消息",
    )
    if send_after != 201:
        raise AssertionError(f"Redis 恢复后消息发送失败: {send_after} {body}")
    return {
        "auth_status_during_outage": auth_status,
        "send_status_during_outage": send_status,
        "ready_status_during_outage": ready_status,
        "recovery_ms": round((time.monotonic() - recovery_started) * 1000, 2),
        "message_after_recovery": send_after,
    }


def restore_containers(containers: list[str]) -> None:
    """确保故障注入结束后所有容器均已启动。"""
    for container in containers:
        try:
            docker_container(container, "start")
        except Exception:
            pass
    for container in containers:
        try:
            wait_container_healthy(container)
        except Exception:
            pass


def run(args: argparse.Namespace) -> dict[str, Any]:
    """执行基础设施故障注入并返回可公开的结果摘要。"""
    suffix = (
        datetime.now(UTC).strftime("%m%d%H%M%S")
        + f"{time.time_ns() % 1000:03d}"
    )
    report_dir = Path(args.output_dir)
    report_dir.mkdir(parents=True, exist_ok=True)
    containers = [
        f"{args.container_prefix}rabbitmq",
        f"{args.container_prefix}redis",
    ]

    with sync_playwright() as playwright:
        api = playwright.request.new_context(base_url=args.api_url)
        try:
            sender = create_user(
                api,
                f"fault_a_{suffix}",
                f"故障甲{suffix}",
            )
            receiver = create_user(
                api,
                f"fault_b_{suffix}",
                f"故障乙{suffix}",
            )
            conversation_id = prepare_conversation(api, sender, receiver)
            baseline_status, baseline_body = send_message(
                api,
                sender["token"],
                conversation_id,
                f"fault-baseline-{suffix}",
                "故障注入前置消息",
            )
            if baseline_status != 201:
                raise AssertionError(f"故障注入前置消息失败: {baseline_status} {baseline_body}")

            result = {
                "rabbitmq": rabbitmq_fault(
                    api,
                    sender,
                    conversation_id,
                    args.api_url,
                    containers[0],
                ),
                "redis": redis_fault(
                    api,
                    sender,
                    conversation_id,
                    args.api_url,
                    containers[1],
                ),
                "mysql": {
                    "status": "skipped",
                    "reason": "MySQL 使用宿主机服务，不在 Docker 故障脚本中自动停止",
                },
                "conversation_id": conversation_id,
            }
            (report_dir / "result.json").write_text(
                json.dumps(result, ensure_ascii=False, indent=2),
                encoding="utf-8",
            )
            return result
        finally:
            restore_containers(containers)
            api.dispose()


def parse_args() -> argparse.Namespace:
    """读取故障注入测试命令行参数。"""
    parser = argparse.ArgumentParser(description="验证 RelayIM 基础设施故障恢复")
    parser.add_argument(
        "--api-url",
        default=os.getenv(
            "RELAYIM_E2E_API_URL",
            "http://127.0.0.1:8081",
        ),
    )
    parser.add_argument(
        "--container-prefix",
        default=os.getenv("RELAYIM_CONTAINER_PREFIX", "relayim-"),
    )
    parser.add_argument(
        "--output-dir",
        default=os.getenv(
            "RELAYIM_E2E_OUTPUT_DIR",
            ".tmp/infrastructure-fault-e2e",
        ),
    )
    return parser.parse_args()


if __name__ == "__main__":
    print(json.dumps(run(parse_args()), ensure_ascii=False, indent=2))
