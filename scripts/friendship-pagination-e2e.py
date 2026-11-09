"""验证好友和申请分页的浏览器加载更多交互。

真实分页边界由 Go + MySQL 集成测试覆盖；本脚本只创建一个真实登录账号，
通过契约一致的 20+5 条响应验证前端不会一次渲染完整列表。
"""

from __future__ import annotations

import argparse
import json
import os
import time
from datetime import UTC, datetime
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


def ensure_user(api: APIRequestContext, username: str, nickname: str) -> None:
    status, body = api_request(
        api,
        "POST",
        "/auth/register",
        {
            "username": username,
            "password": PASSWORD,
            "confirm_password": PASSWORD,
            "nickname": nickname,
        },
    )
    if status not in (201, 409, 429):
        raise RuntimeError(f"创建分页验收用户失败: {status} {body}")


def user_payload(index: int) -> dict[str, str]:
    return {
        "id": str(9000000000000000000 + index),
        "username": f"page_user_{index:02d}",
        "nickname": f"分页联系人 {index:02d}",
        "avatar_url": "",
    }


def friend_payload(index: int) -> dict[str, Any]:
    return {
        "friendship_id": str(8000000000000000000 + index),
        "user": user_payload(index),
        "remark": "",
        "blocked_by_me": False,
    }


def request_payload(index: int) -> dict[str, Any]:
    return {
        "id": str(7000000000000000000 + index),
        "user": user_payload(index),
        "status": 1,
        "remark": f"分页申请理由 {index:02d}",
        "created_at": "2026-11-09T12:00:00Z",
    }


def page_response(
    route: Route,
    field: str,
    item_factory: Any,
) -> None:
    query = parse_qs(urlparse(route.request.url).query)
    before_id = query.get("before_id", [""])[0]
    start = PAGE_SIZE if before_id else 0
    end = TOTAL_SIZE if before_id else PAGE_SIZE
    items = [item_factory(index) for index in range(start + 1, end + 1)]
    has_more = not before_id
    route.fulfill(
        status=200,
        content_type="application/json",
        body=json.dumps(
            {
                field: items,
                "next_cursor": (
                    str(8000000000000000000 + PAGE_SIZE)
                    if field == "friends" and has_more
                    else str(7000000000000000000 + PAGE_SIZE)
                    if has_more
                    else ""
                ),
                "has_more": has_more,
            },
            ensure_ascii=False,
        ),
    )


def run(args: argparse.Namespace) -> dict[str, Any]:
    suffix = (
        datetime.now(UTC).strftime("%m%d%H%M%S")
        + f"{time.time_ns() % 1000:03d}"
    )
    username = "friendship_pagination_e2e"
    nickname = f"分页验收主人{suffix}"
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
            ensure_user(api, username, nickname)

            page.route(
                "**/api/v1/friends?*",
                lambda route: page_response(route, "friends", friend_payload),
            )
            page.route(
                "**/api/v1/friend-requests/received?*",
                lambda route: page_response(
                    route,
                    "requests",
                    request_payload,
                ),
            )
            page.route(
                "**/api/v1/friend-requests/sent?*",
                lambda route: route.fulfill(
                    status=200,
                    content_type="application/json",
                    body=json.dumps(
                        {
                            "requests": [],
                            "next_cursor": "",
                            "has_more": False,
                        }
                    ),
                ),
            )
            page.route(
                "**/api/v1/user-blocks",
                lambda route: route.fulfill(
                    status=200,
                    content_type="application/json",
                    body=json.dumps({"blocked_users": []}),
                ),
            )

            page.goto("/login", wait_until="networkidle")
            page.get_by_label("用户名").fill(username)
            page.get_by_label("密码").fill(PASSWORD)
            page.get_by_role("button", name="进入 GinChat").click()
            page.wait_for_url("**/")
            page.get_by_role("button", name="好友", exact=True).click()

            dialog = page.locator(".friends-dialog")
            expect(dialog.locator(".friend-row")).to_have_count(PAGE_SIZE)
            expect(
                dialog.locator(".friends-tabs button").filter(has_text="好友")
            ).to_contain_text("20+")
            dialog.get_by_role(
                "button",
                name="加载更多好友",
                exact=True,
            ).click()
            expect(dialog.locator(".friend-row")).to_have_count(TOTAL_SIZE)

            dialog.locator(".friends-tabs button").filter(
                has_text="申请"
            ).click()
            expect(dialog.locator(".request-row")).to_have_count(PAGE_SIZE)
            dialog.get_by_role(
                "button",
                name="加载更多申请",
                exact=True,
            ).click()
            expect(dialog.locator(".request-row")).to_have_count(TOTAL_SIZE)

            screenshot = report_dir / "friendship-pagination.png"
            page.screenshot(path=str(screenshot), full_page=True)
            result = {
                "friend_page_sizes": [PAGE_SIZE, TOTAL_SIZE - PAGE_SIZE],
                "request_page_sizes": [PAGE_SIZE, TOTAL_SIZE - PAGE_SIZE],
                "rendered_friend_count": TOTAL_SIZE,
                "rendered_request_count": TOTAL_SIZE,
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
    parser = argparse.ArgumentParser(description="验证好友与申请列表分页")
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
            ".tmp/friendship-pagination-e2e",
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
