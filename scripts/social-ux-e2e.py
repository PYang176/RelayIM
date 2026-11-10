"""验证独立添加好友入口、头像资料、双击私聊和单向删除关系。"""

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


def create_user(
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
            "device_name": "社交体验验收",
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
    requester: dict[str, Any],
    recipient: dict[str, Any],
) -> None:
    api_ok(
        api,
        "POST",
        "/friend-requests",
        token=requester["token"],
        payload={
            "target_user_id": recipient["id"],
            "remark": "社交体验验收好友",
        },
    )
    received = api_ok(
        api,
        "GET",
        "/friend-requests/received",
        token=recipient["token"],
    )
    request = next(
        item
        for item in received["requests"]
        if str(item["user"]["id"]) == requester["id"]
    )
    api_ok(
        api,
        "POST",
        f"/friend-requests/{request['id']}/accept",
        token=recipient["token"],
    )


def login_ui(page: Page, username: str) -> None:
    page.goto("/login", wait_until="networkidle")
    page.get_by_label("用户名").fill(username)
    page.get_by_label("密码").fill(PASSWORD)
    page.get_by_role("button", name="进入 RelayIM").click()
    page.wait_for_url("**/")
    expect(page.locator(".rail-avatar")).to_be_visible()
    expect(page.locator(".rail-connection.connected")).to_be_visible(
        timeout=10_000
    )


def open_contacts(page: Page):
    page.get_by_role("button", name="联系人").click()
    dialog = page.locator('.friends-dialog[aria-labelledby="friends-title"]')
    expect(dialog).to_be_visible()
    return dialog


def run(args: argparse.Namespace) -> dict[str, Any]:
    suffix = (
        datetime.now(UTC).strftime("%m%d%H%M%S")
        + f"{time.time_ns() % 1000:03d}"
    )
    alice_name = f"ux_a_{suffix}"
    bob_name = f"ux_b_{suffix}"
    carol_name = f"ux_c_{suffix}"
    alice_nickname = f"体验阿岚{suffix}"
    bob_nickname = f"体验北辰{suffix}"
    carol_nickname = f"体验小禾{suffix}"
    report_dir = Path(args.output_dir)
    report_dir.mkdir(parents=True, exist_ok=True)

    with sync_playwright() as playwright:
        api = playwright.request.new_context(base_url=args.api_url)
        browser = playwright.chromium.launch(
            executable_path=args.chrome_path,
            headless=True,
        )
        alice_context = browser.new_context(
            base_url=args.frontend_url,
            viewport={"width": 1440, "height": 960},
        )
        bob_context = browser.new_context(
            base_url=args.frontend_url,
            viewport={"width": 1440, "height": 960},
        )
        mobile_context = browser.new_context(
            base_url=args.frontend_url,
            viewport={"width": 390, "height": 844},
        )
        alice_page = alice_context.new_page()
        bob_page = bob_context.new_page()
        mobile_page = mobile_context.new_page()
        try:
            alice = create_user(api, alice_name, alice_nickname)
            bob = create_user(api, bob_name, bob_nickname)
            create_user(api, carol_name, carol_nickname)
            prepare_friendship(api, alice, bob)

            login_ui(alice_page, alice_name)
            login_ui(bob_page, bob_name)

            dialog = open_contacts(alice_page)
            expect(
                dialog.get_by_role("button", name="添加好友", exact=True)
            ).to_be_visible()
            friend_row = dialog.locator(".friend-row").filter(
                has_text=bob_nickname
            )
            expect(friend_row).to_be_visible()

            friend_row.get_by_role(
                "button",
                name=f"查看{bob_nickname}的资料",
            ).click()
            profile_dialog = alice_page.locator(".user-profile-dialog")
            expect(profile_dialog).to_be_visible()
            expect(profile_dialog).to_contain_text(bob_nickname)
            profile_dialog.get_by_role(
                "button",
                name="关闭用户资料",
            ).click()

            dialog.get_by_role("button", name="添加好友", exact=True).click()
            expect(dialog.locator("#add-friend-title")).to_be_visible()
            dialog.get_by_label("搜索用户名或昵称").fill(carol_name)
            dialog.get_by_role("button", name="搜索", exact=True).click()
            expect(
                dialog.locator(".search-result-row").filter(
                    has_text=carol_nickname
                )
            ).to_be_visible()
            dialog.get_by_role("button", name="返回联系人").click()

            friend_row = dialog.locator(".friend-row").filter(
                has_text=bob_nickname
            )
            friend_row.dblclick()
            expect(alice_page.locator(".conversation-header")).to_contain_text(
                bob_nickname
            )

            bob_conversation = bob_page.locator(".conversation-item").filter(
                has_text=alice_nickname
            )
            expect(bob_conversation).to_be_visible(timeout=10_000)

            alice_page.get_by_label("消息内容").fill("单向好友体验消息")
            alice_page.get_by_role("button", name="发送", exact=True).click()
            bob_conversation.click()
            expect(bob_page.locator("#message-log")).to_contain_text(
                "单向好友体验消息",
                timeout=10_000,
            )
            expect(bob_page.locator(".conversation-header")).to_contain_text(
                "最后序号 1",
                timeout=10_000,
            )
            bob_conversations = api_ok(
                api,
                "GET",
                "/conversations",
                token=bob["token"],
            )
            conversation = next(
                item
                for item in bob_conversations["conversations"]
                if item["title"] == alice_nickname
            )

            bob_dialog = open_contacts(bob_page)
            bob_friend_row = bob_dialog.locator(".friend-row").filter(
                has_text=alice_nickname
            )
            expect(bob_friend_row).to_be_visible()

            alice_dialog = open_contacts(alice_page)
            alice_friend_row = alice_dialog.locator(".friend-row").filter(
                has_text=bob_nickname
            )
            alice_friend_row.get_by_role(
                "button",
                name="删除",
                exact=True,
            ).click()
            alice_friend_row.get_by_role(
                "button",
                name="确认",
                exact=True,
            ).click()
            expect(
                alice_dialog.locator(".friend-row").filter(
                    has_text=bob_nickname
                )
            ).to_have_count(0, timeout=10_000)

            expect(bob_friend_row).to_contain_text(
                "双方当前不是好友",
                timeout=10_000,
            )
            expect(bob_page.locator(".relationship-warning")).to_contain_text(
                "无法继续发送消息",
            )
            expect(bob_page.get_by_label("消息内容")).to_be_disabled()

            status, body = api_request(
                api,
                "POST",
                f"/conversations/{conversation['id']}/messages",
                token=bob["token"],
                payload={
                    "client_message_id": f"not-friends-{suffix}",
                    "content": "不应发送成功",
                },
            )
            if status != 403 or "不是对方的好友" not in body.get("error", ""):
                raise AssertionError(f"单向删除发送状态 = {status}: {body}")

            screenshot = report_dir / "social-ux.png"
            bob_page.screenshot(path=str(screenshot), full_page=True)
            login_ui(mobile_page, alice_name)
            expect(mobile_page.locator(".app-rail")).to_be_visible()
            expect(mobile_page.get_by_label("消息内容")).to_be_visible()
            mobile_screenshot = report_dir / "social-ux-mobile.png"
            mobile_page.screenshot(path=str(mobile_screenshot), full_page=True)
            result = {
                "alice_id": alice["id"],
                "bob_id": bob["id"],
                "conversation_visible_without_refresh": True,
                "profile_dialog": True,
                "double_click_direct": True,
                "unilateral_delete_status": status,
                "screenshot": str(screenshot),
                "mobile_screenshot": str(mobile_screenshot),
            }
            (report_dir / "result.json").write_text(
                json.dumps(result, ensure_ascii=False, indent=2),
                encoding="utf-8",
            )
            return result
        finally:
            alice_context.close()
            bob_context.close()
            mobile_context.close()
            browser.close()
            api.dispose()


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="验证社交体验修复")
    parser.add_argument(
        "--frontend-url",
        default=os.getenv(
            "RELAYIM_E2E_FRONTEND_URL",
            "http://127.0.0.1:8080",
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
            ".tmp/social-ux-e2e",
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
