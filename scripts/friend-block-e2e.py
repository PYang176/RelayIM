"""验证好友拉黑的界面操作、私聊拦截和取消恢复。"""

from __future__ import annotations

import argparse
import json
import os
import time
from datetime import UTC, datetime
from pathlib import Path
from typing import Any

from playwright.sync_api import APIRequestContext, Page, expect, sync_playwright

PASSWORD = "Demo123456!"


def api_request(
    api: APIRequestContext,
    method: str,
    path: str,
    token: str = "",
    payload: dict[str, Any] | None = None,
) -> Any:
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
        timeout=10_000,
    )
    body = response.text()
    return response.status, json.loads(body) if body else None


def api_ok(
    api: APIRequestContext,
    method: str,
    path: str,
    token: str = "",
    payload: dict[str, Any] | None = None,
) -> Any:
    status, body = api_request(api, method, path, token, payload)
    if status < 200 or status >= 300:
        raise RuntimeError(f"{method} {path} 返回 {status}: {body}")
    return body


def register_and_login(
    api: APIRequestContext,
    username: str,
    nickname: str,
) -> dict[str, Any]:
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
    login = api_ok(
        api,
        "POST",
        "/auth/login",
        payload={
            "username": username,
            "password": PASSWORD,
            "device_name": "好友拉黑验收",
            "platform": 4,
        },
    )
    return {
        "id": str(user["id"]),
        "username": username,
        "nickname": nickname,
        "token": login["token"],
    }


def prepare_friendship(
    api: APIRequestContext,
    alice: dict[str, Any],
    bob: dict[str, Any],
) -> str:
    api_ok(
        api,
        "POST",
        "/friend-requests",
        token=alice["token"],
        payload={"target_user_id": bob["id"], "remark": "拉黑验收好友"},
    )
    received = api_ok(
        api,
        "GET",
        "/friend-requests/received",
        token=bob["token"],
    )
    request = next(
        item
        for item in received["requests"]
        if str(item["user"]["id"]) == alice["id"]
    )
    api_ok(
        api,
        "POST",
        f"/friend-requests/{request['id']}/accept",
        token=bob["token"],
    )
    conversation = api_ok(
        api,
        "POST",
        "/conversations/private",
        token=alice["token"],
        payload={"peer_user_id": bob["id"]},
    )
    return conversation["id"]


def login_ui(page: Page, username: str) -> None:
    page.goto("/login", wait_until="networkidle")
    page.get_by_label("用户名").fill(username)
    page.get_by_label("密码").fill(PASSWORD)
    page.get_by_role("button", name="进入 GinChat").click()
    page.wait_for_url("**/")
    expect(page.locator(".conversation-item").first).to_be_visible()


def send_text(
    api: APIRequestContext,
    token: str,
    conversation_id: str,
    client_message_id: str,
    content: str,
) -> tuple[int, Any]:
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


def run(args: argparse.Namespace) -> dict[str, Any]:
    suffix = (
        datetime.now(UTC).strftime("%m%d%H%M%S")
        + f"{time.time_ns() % 1000:03d}"
    )
    alice_name = f"block_a_{suffix}"
    bob_name = f"block_b_{suffix}"
    alice_nickname = f"拉黑验收甲{suffix}"
    bob_nickname = f"拉黑验收乙{suffix}"
    report_dir = Path(args.output_dir)
    report_dir.mkdir(parents=True, exist_ok=True)

    with sync_playwright() as playwright:
        api = playwright.request.new_context(base_url=args.api_url)
        browser = playwright.chromium.launch(
            executable_path=args.chrome_path,
            headless=True,
        )
        context = browser.new_context(
            base_url=args.frontend_url,
            viewport={"width": 1440, "height": 960},
        )
        page = context.new_page()
        try:
            alice = register_and_login(api, alice_name, alice_nickname)
            bob = register_and_login(api, bob_name, bob_nickname)
            conversation_id = prepare_friendship(api, alice, bob)

            login_ui(page, alice_name)
            page.get_by_role("button", name="好友", exact=True).click()
            dialog = page.locator(".friends-dialog")
            expect(dialog).to_be_visible()

            friend_row = dialog.locator(".friend-row").filter(
                has_text=bob_nickname
            )
            expect(friend_row).to_be_visible()
            friend_row.get_by_role("button", name="拉黑", exact=True).click()
            friend_row.get_by_role(
                "button",
                name="确认拉黑",
                exact=True,
            ).click()
            expect(
                friend_row.get_by_role("button", name="取消拉黑", exact=True)
            ).to_be_visible()

            bob_send_status, bob_send_body = send_text(
                api,
                bob["token"],
                conversation_id,
                f"blocked-b-{suffix}",
                "拉黑后不应发送成功",
            )
            alice_send_status, alice_send_body = send_text(
                api,
                alice["token"],
                conversation_id,
                f"blocked-a-{suffix}",
                "拉黑者也不应绕过限制",
            )
            request_status, request_body = api_request(
                api,
                "POST",
                "/friend-requests",
                token=bob["token"],
                payload={
                    "target_user_id": alice["id"],
                    "remark": "拉黑期间好友申请",
                },
            )
            if (
                bob_send_status != 403
                or alice_send_status != 403
                or request_status != 403
            ):
                raise AssertionError(
                    "拉黑拦截状态不符合预期："
                    f"消息乙={bob_send_status} {bob_send_body}，"
                    f"消息甲={alice_send_status} {alice_send_body}，"
                    f"好友申请={request_status} {request_body}"
                )

            blocked_screenshot = report_dir / "friend-blocked.png"
            page.screenshot(path=str(blocked_screenshot), full_page=True)

            dialog.locator(".friends-tabs button").filter(
                has_text="拉黑"
            ).click()
            blocked_row = dialog.locator(".request-row").filter(
                has_text=bob_nickname
            )
            expect(blocked_row).to_be_visible()
            blocked_row.get_by_role(
                "button",
                name="取消拉黑",
                exact=True,
            ).click()

            dialog.locator(".friends-tabs button").filter(
                has_text="好友"
            ).click()
            friend_row = dialog.locator(".friend-row").filter(
                has_text=bob_nickname
            )
            expect(
                friend_row.get_by_role("button", name="发消息", exact=True)
            ).to_be_visible()

            sent = api_ok(
                api,
                "POST",
                f"/conversations/{conversation_id}/messages",
                token=bob["token"],
                payload={
                    "client_message_id": f"unblocked-{suffix}",
                    "content": "取消拉黑后消息恢复",
                },
            )
            page.get_by_role("button", name="关闭联系人").click()
            page.locator(".conversation-item").filter(
                has_text=bob_nickname
            ).click()
            page.get_by_text("取消拉黑后消息恢复", exact=True).wait_for(
                timeout=10_000
            )
            restored_screenshot = report_dir / "friend-unblocked.png"
            page.screenshot(path=str(restored_screenshot), full_page=True)

            result = {
                "conversation_id": conversation_id,
                "blocked_message_status": bob_send_status,
                "blocker_message_status": alice_send_status,
                "blocked_friend_request_status": request_status,
                "restored_message_seq": sent["seq"],
                "blocked_screenshot": str(blocked_screenshot),
                "restored_screenshot": str(restored_screenshot),
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
    parser = argparse.ArgumentParser(description="验证 GinChat 好友拉黑链路")
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
        "--output-dir",
        default=os.getenv(
            "GINCHAT_E2E_OUTPUT_DIR",
            ".tmp/friend-block-e2e",
        ),
    )
    parser.add_argument(
        "--chrome-path",
        default=os.getenv(
            "GINCHAT_E2E_CHROME_PATH",
            r"C:\Program Files\Google\Chrome\Application\chrome.exe",
        ),
    )
    return parser.parse_args()


if __name__ == "__main__":
    print(json.dumps(run(parse_args()), ensure_ascii=False, indent=2))
