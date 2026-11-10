"""验证消息链路在 Tempo 中形成完整 OpenTelemetry Trace。"""

from __future__ import annotations

import argparse
import json
import os
import time
import uuid
from pathlib import Path
from typing import Any

from playwright.sync_api import APIRequestContext, sync_playwright

PASSWORD = "Demo123456!"
USERNAME = "demo_alice_1110032019"


def api_login(api: APIRequestContext) -> dict[str, Any]:
    response = api.post(
        "/api/v1/auth/login",
        data={
            "username": USERNAME,
            "password": PASSWORD,
            "device_name": "Trace Sender",
            "platform": 1,
        },
    )
    if not response.ok:
        raise RuntimeError(
            f"Trace 验收登录失败: {response.status} {response.text()}"
        )
    return response.json()


def flatten_spans(payload: dict[str, Any]) -> list[dict[str, Any]]:
    spans: list[dict[str, Any]] = []
    for batch in payload.get("batches", []):
        for scope in batch.get("scopeSpans", []):
            spans.extend(scope.get("spans", []))
    return spans


def run(args: argparse.Namespace) -> dict[str, Any]:
    report_dir = Path(args.output_dir)
    report_dir.mkdir(parents=True, exist_ok=True)
    page_errors: list[str] = []

    with sync_playwright() as playwright:
        api = playwright.request.new_context(base_url=args.api_url)
        browser = playwright.chromium.launch(
            executable_path=args.chrome_path,
            headless=True,
        )
        page = browser.new_page(base_url=args.frontend_url)
        page.on("pageerror", lambda error: page_errors.append(str(error)))
        try:
            page.goto("/login", wait_until="networkidle")
            page.get_by_label("用户名").fill(USERNAME)
            page.get_by_label("密码").fill(PASSWORD)
            page.get_by_role("button", name="进入 GinChat").click()
            page.wait_for_url("**/")
            page.locator("#message-input").wait_for(state="attached")
            page.wait_for_timeout(1000)

            sender = api_login(api)
            headers = {"Authorization": f"Bearer {sender['token']}"}
            conversations = api.get(
                "/api/v1/conversations?limit=20",
                headers=headers,
            )
            if not conversations.ok:
                raise RuntimeError(conversations.text())
            conversation = conversations.json()["conversations"][0]
            sent = api.post(
                (
                    "/api/v1/conversations/"
                    + conversation["id"]
                    + "/messages"
                ),
                headers=headers,
                data={
                    "client_message_id": str(uuid.uuid4()),
                    "content": f"OpenTelemetry 验收 {time.time_ns()}",
                },
            )
            if not sent.ok:
                raise RuntimeError(sent.text())
            trace_id = sent.headers.get("x-trace-id", "")
            if len(trace_id) != 32:
                raise AssertionError(f"X-Trace-ID 非法: {trace_id}")

            span_names: list[str] = []
            deadline = time.monotonic() + 12
            while time.monotonic() < deadline:
                trace = api.get(
                    f"{args.tempo_url}/api/traces/{trace_id}",
                    headers={"Accept": "application/json"},
                )
                if trace.ok:
                    span_names = [
                        span.get("name", "") for span in flatten_spans(trace.json())
                    ]
                    if any("ws.delivery" in name for name in span_names):
                        break
                time.sleep(0.5)

            required = [
                "POST /api/v1/conversations/:conversation_id/messages",
                "outbox.publish message.created.v1",
                "rabbitmq.consume message.created.v1",
                "realtime.route",
                "rabbitmq.consume ws.delivery",
            ]
            missing = [name for name in required if name not in span_names]
            if missing:
                raise AssertionError(
                    "Trace 缺少 Span: "
                    + json.dumps(missing, ensure_ascii=False)
                    + "；实际 Span="
                    + json.dumps(span_names, ensure_ascii=False)
                )

            result = {
                "trace_id": trace_id,
                "span_names": span_names,
                "page_errors": page_errors,
            }
            (report_dir / "result.json").write_text(
                json.dumps(result, ensure_ascii=False, indent=2),
                encoding="utf-8",
            )
            return result
        finally:
            browser.close()
            api.dispose()


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="验证 OpenTelemetry 消息链路")
    parser.add_argument(
        "--frontend-url",
        default=os.getenv(
            "GINCHAT_E2E_FRONTEND_URL",
            "http://127.0.0.1:5173",
        ),
    )
    parser.add_argument(
        "--api-url",
        default=os.getenv(
            "GINCHAT_E2E_API_URL",
            "http://127.0.0.1:8081",
        ),
    )
    parser.add_argument(
        "--tempo-url",
        default=os.getenv(
            "GINCHAT_E2E_TEMPO_URL",
            "http://127.0.0.1:3200",
        ),
    )
    parser.add_argument(
        "--chrome-path",
        default=os.getenv(
            "GINCHAT_E2E_CHROME_PATH",
            r"C:\Program Files\Google\Chrome\Application\chrome.exe",
        ),
    )
    parser.add_argument(
        "--output-dir",
        default=".tmp/otel-trace-e2e",
    )
    return parser.parse_args()


if __name__ == "__main__":
    print(json.dumps(run(parse_args()), ensure_ascii=False, indent=2))
