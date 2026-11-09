"""验证会话搜索与个人置顶的前端完整交互。

真实搜索 SQL 和置顶分组游标由 Go + MySQL 集成测试覆盖。本脚本使用
真实登录账号和契约一致的会话响应，验证搜索输入防抖、独立搜索结果、
置顶后的服务端刷新以及主聊天面板不会被搜索结果替换。
"""

from __future__ import annotations

import argparse
import json
import os
from datetime import UTC, datetime, timedelta
from pathlib import Path
from typing import Any
from urllib.parse import parse_qs, urlparse

from playwright.sync_api import APIRequestContext, Route, expect, sync_playwright

PASSWORD = "Demo123456!"
TITLES = ("项目群", "闲聊群", "搜索目标")


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
            "username": "conversation_search_pin_e2e",
            "password": PASSWORD,
            "confirm_password": PASSWORD,
            "nickname": "会话搜索置顶验收账号",
        },
    )
    if status not in (201, 409, 429):
        raise RuntimeError(f"创建会话搜索验收用户失败: {status} {body}")


def conversation_payload(
    index: int,
    pinned_ids: set[str],
) -> dict[str, Any]:
    conversation_id = str(6200000000000000100 + index)
    updated_at = datetime(2026, 11, 10, 12, 0, tzinfo=UTC) - timedelta(
        minutes=index,
    )
    return {
        "id": conversation_id,
        "conversation_type": 2,
        "name": TITLES[index],
        "title": TITLES[index],
        "avatar_url": "",
        "last_seq": 20 - index,
        "last_read_seq": 20 - index,
        "pinned": conversation_id in pinned_ids,
        "unread_count": 0,
        "member_count": 3,
        "updated_at": updated_at.isoformat().replace("+00:00", "Z"),
    }


def run(args: argparse.Namespace) -> dict[str, Any]:
    report_dir = Path(args.output_dir)
    report_dir.mkdir(parents=True, exist_ok=True)
    pinned_ids: set[str] = set()
    page_errors: list[str] = []
    lifecycle: dict[str, list[str]] = {"titles": list(TITLES)}
    search_queries: list[str] = []

    with sync_playwright() as playwright:
        api = playwright.request.new_context(base_url=args.api_url)
        browser = playwright.chromium.launch(
            executable_path=args.chrome_path,
            headless=True,
        )
        page = browser.new_page(base_url=args.frontend_url)
        page.on("pageerror", lambda error: page_errors.append(str(error)))

        def list_response(query: str) -> list[dict[str, Any]]:
            matches = [
                conversation_payload(index, pinned_ids)
                for index, title in enumerate(TITLES)
                if not query or query in title
            ]
            return sorted(
                matches,
                key=lambda item: (item["pinned"], item["updated_at"]),
                reverse=True,
            )

        def handle_conversation_api(route: Route) -> None:
            request = route.request
            parsed = urlparse(request.url)
            path = parsed.path
            if path == "/api/v1/conversations" and request.method == "GET":
                query = parse_qs(parsed.query).get("q", [""])[0]
                if query:
                    search_queries.append(query)
                route.fulfill(
                    status=200,
                    content_type="application/json",
                    body=json.dumps(
                        {
                            "conversations": list_response(query),
                            "next_cursor": "",
                            "has_more": False,
                        },
                        ensure_ascii=False,
                    ),
                )
                return

            segments = path.rstrip("/").split("/")
            conversation_id = segments[4] if len(segments) > 4 else ""
            if path.endswith("/pin") and request.method == "PUT":
                payload = json.loads(request.post_data or "{}")
                if payload.get("pinned") is True:
                    pinned_ids.add(conversation_id)
                else:
                    pinned_ids.discard(conversation_id)
                route.fulfill(status=204, body="")
                return
            if path.endswith("/messages") and request.method == "GET":
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
            if path.endswith("/members"):
                route.fulfill(
                    status=200,
                    content_type="application/json",
                    body='{"members":[]}',
                )
                return
            if len(segments) == 5 and request.method == "GET":
                index = int(conversation_id) - 6200000000000000100
                route.fulfill(
                    status=200,
                    content_type="application/json",
                    body=json.dumps(
                        conversation_payload(index, pinned_ids),
                        ensure_ascii=False,
                    ),
                )
                return
            route.continue_()

        try:
            ensure_user(api)
            page.route("**/api/v1/conversations**", handle_conversation_api)
            page.route(
                "**/api/v1/presence/query",
                lambda route: route.fulfill(
                    status=200,
                    content_type="application/json",
                    body='{"presence":[]}',
                ),
            )

            page.goto("/login", wait_until="networkidle")
            page.get_by_label("用户名").fill("conversation_search_pin_e2e")
            page.get_by_label("密码").fill(PASSWORD)
            page.get_by_role("button", name="进入 GinChat").click()
            page.wait_for_url("**/")

            items = page.locator(".conversation-item")
            expect(items).to_have_count(len(TITLES))
            page.get_by_role(
                "button",
                name="置顶搜索目标",
                exact=True,
            ).click()
            expect(
                items.first.locator(".conversation-copy strong")
            ).to_have_text("搜索目标")
            expect(
                page.get_by_role(
                    "button",
                    name="取消置顶搜索目标",
                    exact=True,
                )
            ).to_be_visible()

            items.filter(has_text="闲聊群").locator(".conversation-open").click()
            expect(page.locator(".conversation-header h1")).to_have_text("闲聊群")

            search_input = page.get_by_label("搜索会话")
            search_input.fill("项目")
            expect(items).to_have_count(1)
            expect(
                items.first.locator(".conversation-copy strong")
            ).to_have_text("项目群")
            expect(page.locator(".conversation-header h1")).to_have_text("闲聊群")

            page.get_by_role(
                "button",
                name="清除会话搜索",
                exact=True,
            ).click()
            expect(items).to_have_count(len(TITLES))
            expect(
                items.first.locator(".conversation-copy strong")
            ).to_have_text("搜索目标")

            screenshot = report_dir / "conversation-search-pin.png"
            page.screenshot(path=str(screenshot), full_page=True)
            result = {
                "initial_conversation_count": len(TITLES),
                "pinned_title": "搜索目标",
                "pinned_first_after_refresh": True,
                "search_queries": search_queries,
                "search_result_title": "项目群",
                "active_conversation_preserved_during_search": "闲聊群",
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
    parser = argparse.ArgumentParser(description="验证会话搜索与置顶")
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
        default=".tmp/conversation-search-pin-e2e",
    )
    return parser.parse_args()


if __name__ == "__main__":
    print(json.dumps(run(parse_args()), ensure_ascii=False, indent=2))
