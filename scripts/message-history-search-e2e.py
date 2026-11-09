"""验证会话聊天记录面板和消息全文搜索交互。

真实 ngram 全文查询由 Go + MySQL 集成测试覆盖。本脚本使用真实登录账号，
拦截契约一致的聊天记录响应，验证最近消息、向前加载、全文搜索和分页。
"""

from __future__ import annotations

import argparse
import json
import os
from pathlib import Path
from typing import Any
from urllib.parse import parse_qs, urlparse

from playwright.sync_api import APIRequestContext, Route, expect, sync_playwright

PASSWORD = "Demo123456!"
CONVERSATION_ID = "6400000000000000001"


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
            "username": "message_history_search_e2e",
            "password": PASSWORD,
            "confirm_password": PASSWORD,
            "nickname": "消息搜索验收账号",
        },
    )
    if status not in (201, 409, 429):
        raise RuntimeError(f"创建消息搜索验收用户失败: {status} {body}")


def conversation_payload() -> dict[str, Any]:
    return {
        "id": CONVERSATION_ID,
        "conversation_type": 2,
        "name": "消息检索验收群",
        "title": "消息检索验收群",
        "avatar_url": "",
        "last_seq": 8,
        "last_read_seq": 8,
        "pinned": False,
        "unread_count": 0,
        "member_count": 3,
        "updated_at": "2026-11-10T12:00:00.000Z",
    }


def history_message(seq: int, content: str) -> dict[str, Any]:
    return {
        "message_id": str(6500000000000000000 + seq),
        "conversation_id": CONVERSATION_ID,
        "sender_id": "6600000000000000001",
        "seq": seq,
        "message_type": 1,
        "content": content,
        "recalled": False,
        "created_at": f"2026-11-10T11:{seq:02d}:00.000Z",
    }


def search_result(seq: int, content: str) -> dict[str, Any]:
    return {
        "message_id": str(6500000000000000000 + seq),
        "conversation_id": CONVERSATION_ID,
        "sender_id": "6600000000000000001",
        "sender_name": "测试成员",
        "seq": seq,
        "content": content,
        "created_at": f"2026-11-10T11:{seq:02d}:00.000Z",
    }


def run(args: argparse.Namespace) -> dict[str, Any]:
    report_dir = Path(args.output_dir)
    report_dir.mkdir(parents=True, exist_ok=True)
    page_errors: list[str] = []
    search_queries: list[str] = []
    history_before_sequences: list[int] = []

    with sync_playwright() as playwright:
        api = playwright.request.new_context(base_url=args.api_url)
        browser = playwright.chromium.launch(
            executable_path=args.chrome_path,
            headless=True,
        )
        page = browser.new_page(base_url=args.frontend_url)
        page.on("pageerror", lambda error: page_errors.append(str(error)))

        def handle_conversations(route: Route) -> None:
            request = route.request
            parsed = urlparse(request.url)
            path = parsed.path
            segments = path.rstrip("/").split("/")
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
            if path.endswith("/messages/search"):
                query = parse_qs(parsed.query)
                search_queries.append(query.get("q", [""])[0])
                before_seq = int(query.get("before_seq", ["0"])[0] or 0)
                if before_seq:
                    messages = [search_result(3, "第三条并发搜索结果")]
                    has_more = False
                    next_before_seq = 0
                else:
                    messages = [
                        search_result(8, "第八条并发搜索结果"),
                        search_result(6, "第六条并发搜索结果"),
                    ]
                    has_more = True
                    next_before_seq = 6
                route.fulfill(
                    status=200,
                    content_type="application/json",
                    body=json.dumps(
                        {
                            "messages": messages,
                            "next_before_seq": next_before_seq,
                            "has_more": has_more,
                        },
                        ensure_ascii=False,
                    ),
                )
                return
            if path.endswith("/messages") and request.method == "GET":
                query = parse_qs(parsed.query)
                before_seq = int(query.get("before_seq", ["0"])[0] or 0)
                if before_seq:
                    history_before_sequences.append(before_seq)
                    messages = [history_message(1, "第一条较早历史消息")]
                    has_more = False
                else:
                    messages = [
                        history_message(7, "第七条最近消息"),
                        history_message(8, "第八条最近消息"),
                    ]
                    has_more = True
                route.fulfill(
                    status=200,
                    content_type="application/json",
                    body=json.dumps(
                        {"messages": messages, "has_more": has_more},
                        ensure_ascii=False,
                    ),
                )
                return
            if path.endswith("/read-states"):
                route.fulfill(
                    status=200,
                    content_type="application/json",
                    body='{"read_states":[]}',
                )
                return
            if path.endswith("/members"):
                route.fulfill(
                    status=200,
                    content_type="application/json",
                    body=(
                        '{"members":[{"user_id":"6600000000000000001",'
                        '"username":"search_member","nickname":"测试成员",'
                        '"avatar_url":"","role":1,'
                        '"joined_at":"2026-11-10T10:00:00Z"}]}'
                    ),
                )
                return
            if len(segments) == 5 and request.method == "GET":
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
            page.route(
                "**/api/v1/presence/query",
                lambda route: route.fulfill(
                    status=200,
                    content_type="application/json",
                    body='{"presence":[]}',
                ),
            )

            page.goto("/login", wait_until="networkidle")
            page.get_by_label("用户名").fill("message_history_search_e2e")
            page.get_by_label("密码").fill(PASSWORD)
            page.get_by_role("button", name="进入 GinChat").click()
            page.wait_for_url("**/")

            page.get_by_role("button", name="聊天记录", exact=True).click()
            dialog = page.locator(".message-history-dialog")
            expect(dialog).to_be_visible()
            expect(dialog.locator(".message-history-entry")).to_have_count(2)
            dialog.get_by_role(
                "button",
                name="加载更早记录",
                exact=True,
            ).click()
            expect(dialog.locator(".message-history-entry")).to_have_count(3)

            dialog.get_by_label("搜索聊天记录").fill("并发")
            dialog.get_by_role("button", name="搜索", exact=True).click()
            expect(
                dialog.locator(".message-history-entry")
            ).to_have_count(2)
            expect(dialog).to_contain_text("测试成员")
            dialog.get_by_role(
                "button",
                name="加载更多搜索结果",
                exact=True,
            ).click()
            expect(
                dialog.locator(".message-history-entry")
            ).to_have_count(3)
            search_screenshot = report_dir / "message-search.png"
            page.screenshot(path=str(search_screenshot), full_page=True)

            dialog.get_by_role(
                "button",
                name="返回最近记录",
                exact=True,
            ).click()
            expect(
                dialog.locator(".message-history-entry")
            ).to_have_count(3)

            history_screenshot = report_dir / "message-history.png"
            page.screenshot(path=str(history_screenshot), full_page=True)
            result = {
                "recent_history_initial_count": 2,
                "recent_history_after_more_count": 3,
                "history_before_sequences": history_before_sequences,
                "search_queries": search_queries,
                "search_initial_count": 2,
                "search_after_more_count": 3,
                "returned_to_recent_count": 3,
                "page_errors": page_errors,
                "history_screenshot": str(history_screenshot),
                "search_screenshot": str(search_screenshot),
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
    parser = argparse.ArgumentParser(description="验证聊天记录与消息搜索")
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
        "--chrome-path",
        default=os.getenv(
            "GINCHAT_E2E_CHROME_PATH",
            r"C:\Program Files\Google\Chrome\Application\chrome.exe",
        ),
    )
    parser.add_argument(
        "--output-dir",
        default=".tmp/message-history-search-e2e",
    )
    return parser.parse_args()


if __name__ == "__main__":
    print(json.dumps(run(parse_args()), ensure_ascii=False, indent=2))
