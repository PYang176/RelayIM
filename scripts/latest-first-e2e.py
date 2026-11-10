"""验证最新优先、历史懒加载和断线重连窗口替换。

脚本要求 Go 服务、前端和基础设施已经启动。测试通过 HTTP 准备真实用户、
好友关系和消息，再用 Chromium 登录接收方页面验证可见消息窗口。
"""

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
INITIAL_MESSAGE_COUNT = 65
LATEST_PAGE_SIZE = 40
MESSAGE_RATE_LIMIT_BATCH = 20
MESSAGE_RATE_LIMIT_WAIT_SECONDS = 11


def api_call(
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
    if not response.ok:
        raise RuntimeError(
            f"{method} {path} 返回 {response.status}: {body}"
        )
    return json.loads(body) if body else None


def register_and_login(
    api: APIRequestContext,
    username: str,
    nickname: str,
    device_name: str,
) -> dict[str, Any]:
    registered = api_call(
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
    login = api_call(
        api,
        "POST",
        "/auth/login",
        payload={
            "username": username,
            "password": PASSWORD,
            "device_name": device_name,
            "platform": 4,
        },
    )
    return {
        "id": str(registered["id"]),
        "username": username,
        "nickname": nickname,
        "token": login["token"],
    }


def add_friend(
    api: APIRequestContext,
    requester: dict[str, Any],
    recipient: dict[str, Any],
) -> None:
    api_call(
        api,
        "POST",
        "/friend-requests",
        token=requester["token"],
        payload={
            "target_user_id": recipient["id"],
            "remark": "最新优先验收好友",
        },
    )
    received = api_call(
        api,
        "GET",
        "/friend-requests/received",
        token=recipient["token"],
    )
    request = next(
        (
            item
            for item in received["requests"]
            if str(item["user"]["id"]) == requester["id"]
        ),
        None,
    )
    if request is None:
        raise RuntimeError("未找到验收好友申请")
    api_call(
        api,
        "POST",
        f"/friend-requests/{request['id']}/accept",
        token=recipient["token"],
    )


def seed_conversation(
    api: APIRequestContext,
    alice: dict[str, Any],
    bob: dict[str, Any],
    suffix: str,
) -> tuple[dict[str, Any], list[dict[str, Any]]]:
    add_friend(api, alice, bob)
    conversation = api_call(
        api,
        "POST",
        "/conversations/private",
        token=alice["token"],
        payload={"peer_user_id": bob["id"]},
    )
    messages = []
    for index in range(1, INITIAL_MESSAGE_COUNT + 1):
        messages.append(
            api_call(
                api,
                "POST",
                f"/conversations/{conversation['id']}/messages",
                token=alice["token"],
                payload={
                    "client_message_id": f"latest-first-{suffix}-{index}",
                    "content": f"最新优先验收消息 {index:03d}",
                },
            )
        )
        if (
            index % MESSAGE_RATE_LIMIT_BATCH == 0
            and index < INITIAL_MESSAGE_COUNT
        ):
            # 发送接口按用户每 10 秒限制 20 条，等待略长于窗口再继续造数。
            time.sleep(MESSAGE_RATE_LIMIT_WAIT_SECONDS)
    return conversation, messages


def login_ui(page: Page, username: str, password: str) -> None:
    page.goto("/login", wait_until="networkidle")
    page.get_by_label("用户名").fill(username)
    page.get_by_label("密码").fill(password)
    page.get_by_role("button", name="进入 RelayIM").click()
    page.wait_for_url("**/")
    expect(page.locator(".conversation-item").first).to_be_visible()


def visible_sequences(page: Page) -> list[int]:
    return [
        int(value)
        for value in page.locator(".message-seq").all_inner_texts()
    ]


def assert_window(
    page: Page,
    expected_count: int,
    expected_first: int,
    expected_last: int,
) -> list[int]:
    expect(page.locator(".message-seq")).to_have_count(
        expected_count,
        timeout=15_000,
    )
    sequences = visible_sequences(page)
    if sequences[0] != expected_first or sequences[-1] != expected_last:
        raise AssertionError(
            "消息窗口不匹配："
            f"实际 {sequences[0]}..{sequences[-1]}，"
            f"预期 {expected_first}..{expected_last}"
        )
    return sequences


def wait_for_reconnect(page: Page) -> None:
    page.evaluate(
        """() => {
            for (const socket of window.__relayimSockets ?? []) {
                socket.close()
            }
        }"""
    )
    page.wait_for_function(
        """() => {
            const text =
                document.querySelector('.connection-state')?.textContent ?? ''
            return text.includes('断开') || text.includes('重连')
        }""",
        timeout=15_000,
    )
    page.context.set_offline(False)
    expect(
        page.locator(".connection-state")
    ).to_contain_text("实时连接正常", timeout=30_000)


def run(args: argparse.Namespace) -> dict[str, Any]:
    suffix = (
        datetime.now(UTC).strftime("%m%d%H%M%S")
        + f"{time.time_ns() % 1000:03d}"
    )
    alice_username = f"latest_a_{suffix}"
    bob_username = f"latest_b_{suffix}"
    alice_nickname = f"验收发送方{suffix}"
    bob_nickname = f"验收接收方{suffix}"
    report_dir = Path(args.output_dir)
    report_dir.mkdir(parents=True, exist_ok=True)

    with sync_playwright() as playwright:
        api = playwright.request.new_context(base_url=args.api_url)
        browser = playwright.chromium.launch(
            executable_path=args.chrome_path,
            headless=True,
        )
        browser_context = browser.new_context(
            base_url=args.frontend_url,
            viewport={"width": 1440, "height": 960},
        )
        page = browser_context.new_page()
        page.add_init_script(
            """(() => {
                const NativeWebSocket = window.WebSocket
                window.__relayimSockets = []
                window.WebSocket = class extends NativeWebSocket {
                    constructor(...args) {
                        super(...args)
                        window.__relayimSockets.push(this)
                    }
                }
            })()"""
        )

        try:
            alice = register_and_login(
                api,
                alice_username,
                alice_nickname,
                "最新优先验收发送方",
            )
            bob = register_and_login(
                api,
                bob_username,
                bob_nickname,
                "最新优先验收接收方",
            )
            conversation, _ = seed_conversation(
                api,
                alice,
                bob,
                suffix,
            )

            login_ui(page, bob_username, PASSWORD)
            page.locator(".conversation-item").filter(
                has_text=alice_nickname
            ).click()
            page.get_by_text(
                f"最新优先验收消息 {INITIAL_MESSAGE_COUNT:03d}",
                exact=True,
            ).wait_for(timeout=15_000)

            latest_sequences = assert_window(
                page,
                LATEST_PAGE_SIZE,
                INITIAL_MESSAGE_COUNT - LATEST_PAGE_SIZE + 1,
                INITIAL_MESSAGE_COUNT,
            )

            page.get_by_role("button", name="加载更早消息").click()
            history_sequences = assert_window(
                page,
                INITIAL_MESSAGE_COUNT,
                1,
                INITIAL_MESSAGE_COUNT,
            )

            online_message = api_call(
                api,
                "POST",
                f"/conversations/{conversation['id']}/messages",
                token=alice["token"],
                payload={
                    "client_message_id": f"latest-first-{suffix}-online",
                    "content": "最新优先验收消息 066 在线",
                },
            )
            page.get_by_text("最新优先验收消息 066 在线", exact=True).wait_for(
                timeout=15_000
            )
            expect(page.locator(".message-seq")).to_have_count(
                INITIAL_MESSAGE_COUNT + 1
            )

            page.context.set_offline(True)
            missed_message = api_call(
                api,
                "POST",
                f"/conversations/{conversation['id']}/messages",
                token=alice["token"],
                payload={
                    "client_message_id": f"latest-first-{suffix}-missed",
                    "content": "最新优先验收消息 067 断线期间",
                },
            )
            wait_for_reconnect(page)
            page.get_by_text(
                "最新优先验收消息 067 断线期间",
                exact=True,
            ).wait_for(timeout=20_000)

            reconnect_sequences = assert_window(
                page,
                LATEST_PAGE_SIZE,
                INITIAL_MESSAGE_COUNT + 2 - LATEST_PAGE_SIZE + 1,
                INITIAL_MESSAGE_COUNT + 2,
            )
            screenshot = report_dir / "latest-first-reconnect.png"
            page.screenshot(path=str(screenshot), full_page=True)

            result = {
                "conversation_id": conversation["id"],
                "initial_message_count": INITIAL_MESSAGE_COUNT,
                "initial_window": {
                    "count": len(latest_sequences),
                    "first_seq": latest_sequences[0],
                    "last_seq": latest_sequences[-1],
                },
                "history_window": {
                    "count": len(history_sequences),
                    "first_seq": history_sequences[0],
                    "last_seq": history_sequences[-1],
                },
                "realtime_message_seq": online_message["seq"],
                "missed_message_seq": missed_message["seq"],
                "reconnect_window": {
                    "count": len(reconnect_sequences),
                    "first_seq": reconnect_sequences[0],
                    "last_seq": reconnect_sequences[-1],
                },
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
    parser = argparse.ArgumentParser(
        description="验证 RelayIM 最新优先、历史懒加载和断线重连窗口"
    )
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
            ".tmp/latest-first-e2e",
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
    print(
        json.dumps(
            run(parse_args()),
            ensure_ascii=False,
            indent=2,
        )
    )
