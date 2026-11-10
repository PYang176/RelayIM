"""验证聊天附件在浏览器端的大小和空文件预检。"""

from __future__ import annotations

import argparse
import json
import os
from pathlib import Path
from typing import Any
from urllib.parse import urlparse

from playwright.sync_api import APIRequestContext, Route, expect, sync_playwright

PASSWORD = "Demo123456!"
MAX_ATTACHMENT_BYTES = 25 * 1024 * 1024
CONVERSATION_ID = "6800000000000000001"


def api_request(
    api: APIRequestContext,
    method: str,
    path: str,
    payload: dict[str, Any] | None = None,
) -> tuple[int, Any]:
    data = json.dumps(payload, ensure_ascii=False) if payload else None
    headers = {"Content-Type": "application/json"} if payload else {}
    response = api.fetch(
        f"/api/v1{path}",
        method=method,
        headers=headers,
        data=data,
    )
    body = response.text()
    return response.status, json.loads(body) if body else None


def ensure_user(api: APIRequestContext) -> None:
    status, body = api_request(
        api,
        "POST",
        "/auth/register",
        {
            "username": "attachment_size_e2e",
            "password": PASSWORD,
            "confirm_password": PASSWORD,
            "nickname": "附件限制验收用户",
        },
    )
    if status not in (201, 409, 429):
        raise RuntimeError(f"创建附件限制验收用户失败: {status} {body}")


def conversation_payload() -> dict[str, Any]:
    return {
        "id": CONVERSATION_ID,
        "conversation_type": 1,
        "name": "",
        "title": "附件限制验收会话",
        "peer_user_id": "6800000000000000099",
        "avatar_url": "",
        "last_seq": 0,
        "last_read_seq": 0,
        "pinned": False,
        "unread_count": 0,
        "member_count": 2,
        "updated_at": "2026-11-10T12:00:00.000Z",
    }


def run(args: argparse.Namespace) -> dict[str, Any]:
    report_dir = Path(args.output_dir)
    report_dir.mkdir(parents=True, exist_ok=True)
    attachment_requests = 0
    page_errors: list[str] = []

    with sync_playwright() as playwright:
        api = playwright.request.new_context(base_url=args.api_url)
        browser = playwright.chromium.launch(
            executable_path=args.chrome_path,
            headless=True,
        )
        page = browser.new_page(base_url=args.frontend_url)
        page.on("pageerror", lambda error: page_errors.append(str(error)))

        def count_attachment_request(route: Route) -> None:
            nonlocal attachment_requests
            attachment_requests += 1
            route.fulfill(
                status=400,
                content_type="application/json",
                body='{"error":"不应上传超限文件"}',
            )

        def handle_conversations(route: Route) -> None:
            path = urlparse(route.request.url).path
            if path == "/api/v1/conversations":
                route.fulfill(
                    status=200,
                    content_type="application/json",
                    body=json.dumps(
                        {
                            "conversations": [conversation_payload()],
                            "next_cursor": "",
                            "has_more": False,
                        },
                        ensure_ascii=False,
                    ),
                )
                return
            if path.endswith("/messages"):
                route.fulfill(
                    status=200,
                    content_type="application/json",
                    body='{"messages":[],"has_more":false}',
                )
                return
            if path.endswith("/read-states"):
                route.fulfill(
                    status=200,
                    content_type="application/json",
                    body='{"read_states":[]}',
                )
                return
            if len(path.rstrip("/").split("/")) == 5:
                route.fulfill(
                    status=200,
                    content_type="application/json",
                    body=json.dumps(
                        conversation_payload(),
                        ensure_ascii=False,
                    ),
                )
                return
            route.continue_()

        try:
            ensure_user(api)
            page.route("**/api/v1/conversations**", handle_conversations)
            page.route("**/api/v1/attachments", count_attachment_request)
            page.route(
                "**/api/v1/presence/query",
                lambda route: route.fulfill(
                    status=200,
                    content_type="application/json",
                    body='{"presence":[]}',
                ),
            )

            page.goto("/login", wait_until="networkidle")
            page.get_by_label("用户名").fill("attachment_size_e2e")
            page.get_by_label("密码").fill(PASSWORD)
            page.get_by_role("button", name="进入 RelayIM").click()
            page.wait_for_url("**/")

            file_input = page.locator('.composer input[type="file"]')
            file_input.set_input_files(
                {
                    "name": "too-large.bin",
                    "mimeType": "application/octet-stream",
                    "buffer": b"0" * (MAX_ATTACHMENT_BYTES + 1),
                }
            )
            expect(page.locator(".composer-error")).to_have_text(
                "附件不能超过 25 MB"
            )
            expect(
                page.get_by_role("button", name="图片/文件")
            ).to_be_enabled()

            file_input.set_input_files(
                {
                    "name": "empty.txt",
                    "mimeType": "text/plain",
                    "buffer": b"",
                }
            )
            expect(page.locator(".composer-error")).to_have_text(
                "不能上传空文件"
            )
            if attachment_requests != 0:
                raise AssertionError(
                    f"无效文件不应发起上传请求，实际 {attachment_requests} 次"
                )

            screenshot = report_dir / "attachment-size-limit.png"
            page.screenshot(path=str(screenshot), full_page=True)
            result = {
                "max_attachment_bytes": MAX_ATTACHMENT_BYTES,
                "oversized_file_blocked": True,
                "empty_file_blocked": True,
                "attachment_requests": attachment_requests,
                "page_errors": page_errors,
                "screenshot": str(screenshot),
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
    parser = argparse.ArgumentParser(description="验证附件大小预检")
    parser.add_argument(
        "--frontend-url",
        default=os.getenv(
            "RELAYIM_E2E_FRONTEND_URL",
            "http://127.0.0.1:5173",
        ),
    )
    parser.add_argument(
        "--api-url",
        default=os.getenv(
            "RELAYIM_E2E_API_URL",
            "http://127.0.0.1:8081",
        ),
    )
    parser.add_argument(
        "--chrome-path",
        default=os.getenv(
            "RELAYIM_E2E_CHROME_PATH",
            r"C:\Program Files\Google\Chrome\Application\chrome.exe",
        ),
    )
    parser.add_argument(
        "--output-dir",
        default=".tmp/attachment-size-limit-e2e",
    )
    return parser.parse_args()


if __name__ == "__main__":
    print(json.dumps(run(parse_args()), ensure_ascii=False, indent=2))
