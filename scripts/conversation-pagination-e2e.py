"""验证会话列表分页、发送后置顶和活跃会话定时对账。

真实游标边界由 Go + MySQL 集成测试覆盖。本脚本创建真实登录账号，
用契约一致的两页会话响应验证浏览器只渲染首屏 20 条、加载更多后
达到 25 条，并通过一次发送消息验证会话排序会更新到顶部。
"""

from __future__ import annotations

import argparse
import json
import os
import time
from datetime import UTC, datetime, timedelta
from pathlib import Path
from typing import Any
from urllib.parse import parse_qs, urlparse

from playwright.sync_api import APIRequestContext, Route, expect, sync_playwright

PASSWORD = "Demo123456!"
PAGE_SIZE = 20
TOTAL_SIZE = 25


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
            "username": "conversation_pagination_e2e",
            "password": PASSWORD,
            "confirm_password": PASSWORD,
            "nickname": "会话分页验收账号",
        },
    )
    if status not in (201, 409, 429):
        raise RuntimeError(f"创建会话分页验收用户失败: {status} {body}")


def conversation_payload(index: int) -> dict[str, Any]:
    updated_at = datetime(2026, 11, 10, 12, 0, tzinfo=UTC) - timedelta(
        minutes=index,
    )
    return {
        "id": str(6000000000000000100 + index),
        "conversation_type": 1,
        "name": "",
        "title": f"分页会话 {index:02d}",
        "peer_user_id": str(9000000000000000100 + index),
        "avatar_url": "",
        "last_seq": 100 - index,
        "last_read_seq": 100 - index,
        "unread_count": 0,
        "member_count": 2,
        "updated_at": updated_at.isoformat().replace("+00:00", "Z"),
    }


def run(args: argparse.Namespace) -> dict[str, Any]:
    report_dir = Path(args.output_dir)
    report_dir.mkdir(parents=True, exist_ok=True)
    single_requests = 0
    conversation_requests: list[str] = []
    page_errors: list[str] = []

    with sync_playwright() as playwright:
        api = playwright.request.new_context(base_url=args.api_url)
        browser = playwright.chromium.launch(
            executable_path=args.chrome_path,
            headless=True,
        )
        page = browser.new_page(base_url=args.frontend_url)
        # 捕获 30 秒业务对账回调，测试时直接执行一次，避免空等完整周期。
        page.add_init_script(
            """
            const nativeSetInterval = window.setInterval.bind(window)
            window.__conversationReconcileCallbacks = []
            window.setInterval = (handler, timeout, ...args) => {
                if (timeout === 30000) {
                    window.__conversationReconcileCallbacks.push(handler)
                }
                return nativeSetInterval(
                    handler,
                    timeout === 30000 ? 60000 : timeout,
                    ...args,
                )
            }
            """
        )
        page.on("pageerror", lambda error: page_errors.append(str(error)))

        def handle_conversation_api(route: Route) -> None:
            nonlocal single_requests
            request = route.request
            parsed = urlparse(request.url)
            path = parsed.path
            conversation_requests.append(f"{request.method} {parsed.path}")
            if path == "/api/v1/conversations" and request.method == "GET":
                query = parse_qs(parsed.query)
                cursor = query.get("cursor", [""])[0]
                start = PAGE_SIZE if cursor else 0
                end = TOTAL_SIZE if cursor else PAGE_SIZE
                has_more = not cursor
                route.fulfill(
                    status=200,
                    content_type="application/json",
                    body=json.dumps(
                        {
                            "conversations": [
                                conversation_payload(index)
                                for index in range(start, end)
                            ],
                            "next_cursor": "conversation-page-2" if has_more else "",
                            "has_more": has_more,
                        },
                        ensure_ascii=False,
                    ),
                )
                return

            segments = path.rstrip("/").split("/")
            conversation_id = segments[4] if len(segments) > 4 else ""
            if path.endswith("/messages") and request.method == "GET":
                route.fulfill(
                    status=200,
                    content_type="application/json",
                    body='{"messages":[],"has_more":false}',
                )
                return
            if path.endswith("/messages") and request.method == "POST":
                route.fulfill(
                    status=200,
                    content_type="application/json",
                    body=json.dumps(
                        {
                            "message_id": "6100000000000000999",
                            "conversation_id": conversation_id,
                            "seq": 999,
                            "created_at": "2026-11-10T13:00:00.000Z",
                            "duplicate": False,
                        }
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
            if len(segments) == 5 and request.method == "GET":
                index = int(conversation_id) - 6000000000000000100
                single_requests += 1
                route.fulfill(
                    status=200,
                    content_type="application/json",
                    body=json.dumps(
                        conversation_payload(index),
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
            page.get_by_label("用户名").fill("conversation_pagination_e2e")
            page.get_by_label("密码").fill(PASSWORD)
            page.get_by_role("button", name="进入 GinChat").click()
            page.wait_for_url("**/")

            conversation_items = page.locator(".conversation-item")
            expect(conversation_items).to_have_count(PAGE_SIZE)
            page.get_by_role(
                "button",
                name="加载更多会话",
                exact=True,
            ).click()
            expect(conversation_items).to_have_count(TOTAL_SIZE)

            selected = conversation_items.last
            selected_title = selected.locator(".conversation-copy strong").inner_text()
            selected.click()
            expect(page.locator(".conversation-item.active")).to_contain_text(
                selected_title
            )
            composer = page.locator("#message-input")
            expect(composer).to_be_enabled()
            composer.fill("验证会话发送后置顶")
            page.get_by_role("button", name="发送", exact=True).click()

            first_title = conversation_items.first.locator(
                ".conversation-copy strong"
            )
            expect(first_title).to_have_text(selected_title)
            page.screenshot(
                path=str(report_dir / "conversation-pagination-before-reconcile.png"),
                full_page=True,
            )
            page.evaluate(
                """
                () => {
                    const callback =
                        window.__conversationReconcileCallbacks.at(-1)
                    if (!callback) {
                        throw new Error('未注册会话对账定时回调')
                    }
                    callback()
                }
                """
            )

            deadline = time.monotonic() + 5
            while single_requests == 0 and time.monotonic() < deadline:
                time.sleep(0.5)
            if single_requests == 0:
                raise AssertionError(
                    "定时对账未调用单会话接口；已拦截请求="
                    + json.dumps(conversation_requests, ensure_ascii=False)
                    + "；页面错误="
                    + json.dumps(page_errors, ensure_ascii=False)
                )

            screenshot = report_dir / "conversation-pagination.png"
            page.screenshot(path=str(screenshot), full_page=True)
            result = {
                "first_page_size": PAGE_SIZE,
                "second_page_size": TOTAL_SIZE - PAGE_SIZE,
                "rendered_conversation_count": TOTAL_SIZE,
                "moved_to_top_title": selected_title,
                "single_conversation_reconcile_requests": single_requests,
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
    parser = argparse.ArgumentParser(description="验证会话列表分页与置顶")
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
        default=".tmp/conversation-pagination-e2e",
    )
    return parser.parse_args()


if __name__ == "__main__":
    print(json.dumps(run(parse_args()), ensure_ascii=False, indent=2))
