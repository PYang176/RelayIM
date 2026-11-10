"""验证好友申请理由的必填校验、界面输入和列表展示。"""

from __future__ import annotations

import argparse
import json
import os
import time
from datetime import UTC, datetime
from pathlib import Path
from typing import Any

from playwright.sync_api import APIRequestContext, expect, sync_playwright

PASSWORD = "Demo123456!"


def request_json(
    api: APIRequestContext,
    method: str,
    path: str,
    token: str = "",
    payload: dict[str, Any] | None = None,
) -> tuple[int, Any]:
    headers = {}
    data = None
    if payload is not None:
        headers["Content-Type"] = "application/json"
        data = json.dumps(payload, ensure_ascii=False)
    if token:
        headers["Authorization"] = f"Bearer {token}"
    response = api.fetch(
        f"/api/v1{path}",
        method=method,
        headers=headers,
        data=data,
    )
    body = response.text()
    return response.status, json.loads(body) if body else None


def create_user(
    api: APIRequestContext,
    username: str,
    nickname: str,
) -> dict[str, Any]:
    status, user = request_json(
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
    if status != 201:
        raise RuntimeError(f"注册用户失败: {status} {user}")
    status, login = request_json(
        api,
        "POST",
        "/auth/login",
        payload={
            "username": username,
            "password": PASSWORD,
            "device_name": "申请理由验收",
            "platform": 4,
        },
    )
    if status != 200:
        raise RuntimeError(f"登录用户失败: {status} {login}")
    return {
        "id": str(user["id"]),
        "username": username,
        "nickname": nickname,
        "token": login["token"],
    }


def run(args: argparse.Namespace) -> dict[str, Any]:
    suffix = (
        datetime.now(UTC).strftime("%m%d%H%M%S")
        + f"{time.time_ns() % 1000:03d}"
    )
    sender_name = f"reason_a_{suffix}"
    recipient_name = f"reason_b_{suffix}"
    sender_nickname = f"理由验收甲{suffix}"
    recipient_nickname = f"理由验收乙{suffix}"
    reason = "我们在同一个项目组，想和你沟通联调安排"
    report_dir = Path(args.output_dir)
    report_dir.mkdir(parents=True, exist_ok=True)

    with sync_playwright() as playwright:
        api = playwright.request.new_context(base_url=args.api_url)
        browser = playwright.chromium.launch(
            executable_path=args.chrome_path,
            headless=True,
        )
        page = browser.new_page(base_url=args.frontend_url)
        try:
            sender = create_user(api, sender_name, sender_nickname)
            recipient = create_user(api, recipient_name, recipient_nickname)

            empty_status, empty_body = request_json(
                api,
                "POST",
                "/friend-requests",
                token=sender["token"],
                payload={
                    "target_user_id": recipient["id"],
                    "remark": " ",
                },
            )
            if empty_status != 400:
                raise AssertionError(
                    f"空理由应返回 400，实际 {empty_status}: {empty_body}"
                )

            page.goto("/login", wait_until="networkidle")
            page.get_by_label("用户名").fill(sender_name)
            page.get_by_label("密码").fill(PASSWORD)
            page.get_by_role("button", name="进入 RelayIM").click()
            page.wait_for_url("**/")
            page.get_by_role("button", name="好友", exact=True).click()

            dialog = page.locator(".friends-dialog")
            dialog.get_by_label("搜索用户").fill(recipient_name)
            dialog.get_by_role("button", name="搜索", exact=True).click()
            result_row = dialog.locator(".request-row").filter(
                has_text=recipient_nickname
            )
            expect(result_row).to_be_visible()
            result_row.get_by_role("button", name="添加", exact=True).click()
            result_row.get_by_label("好友申请理由").fill(reason)
            result_row.get_by_role(
                "button",
                name="发送申请",
                exact=True,
            ).click()
            expect(result_row).to_have_count(0, timeout=10_000)

            received_status, received = request_json(
                api,
                "GET",
                "/friend-requests/received",
                token=recipient["token"],
            )
            if received_status != 200:
                raise RuntimeError(f"查询申请失败: {received_status} {received}")
            application = next(
                item
                for item in received["requests"]
                if str(item["user"]["id"]) == sender["id"]
            )
            if application["remark"] != reason:
                raise AssertionError(
                    f"申请理由不一致: {application['remark']!r}"
                )

            dialog.locator(".friends-tabs button").filter(
                has_text="申请"
            ).click()
            dialog.get_by_role("button", name="发出", exact=True).click()
            expect(dialog.get_by_text(reason, exact=True)).to_be_visible()

            screenshot = report_dir / "friend-request-reason.png"
            page.screenshot(path=str(screenshot), full_page=True)
            result = {
                "empty_reason_status": empty_status,
                "request_id": application["id"],
                "request_remark": application["remark"],
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
    parser = argparse.ArgumentParser(description="验证好友申请理由必填链路")
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
        "--output-dir",
        default=os.getenv(
            "RELAYIM_E2E_OUTPUT_DIR",
            ".tmp/friend-request-reason-e2e",
        ),
    )
    parser.add_argument(
        "--chrome-path",
        default=os.getenv(
            "RELAYIM_E2E_CHROME_PATH",
            r"C:\Program Files\Google\Chrome\Application\chrome.exe",
        ),
    )
    return parser.parse_args()


if __name__ == "__main__":
    print(json.dumps(run(parse_args()), ensure_ascii=False, indent=2))
