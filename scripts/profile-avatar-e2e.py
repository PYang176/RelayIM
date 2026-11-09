"""验证资料头像上传、保存后保留和移除闭环。"""

from __future__ import annotations

import argparse
import base64
import json
import os
from pathlib import Path
from typing import Any

from playwright.sync_api import APIRequestContext, expect, sync_playwright

PASSWORD = "Demo123456!"
PNG_BYTES = base64.b64decode(
    "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk"
    "+A8AAQUBAScY42YAAAAASUVORK5CYII="
)


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
            "username": "profile_avatar_e2e",
            "password": PASSWORD,
            "confirm_password": PASSWORD,
            "nickname": "头像验收用户",
        },
    )
    if status not in (201, 409, 429):
        raise RuntimeError(f"创建头像验收用户失败: {status} {body}")


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
            ensure_user(api)
            page.goto("/login", wait_until="networkidle")
            page.get_by_label("用户名").fill("profile_avatar_e2e")
            page.get_by_label("密码").fill(PASSWORD)
            page.get_by_role("button", name="进入 GinChat").click()
            page.wait_for_url("**/")

            page.get_by_role("button", name="资料", exact=True).click()
            dialog = page.locator(".profile-dialog")
            expect(dialog).to_be_visible()
            expect(dialog).not_to_contain_text("头像地址")

            file_input = dialog.locator('input[type="file"]')
            file_input.set_input_files(
                {
                    "name": "avatar.png",
                    "mimeType": "image/png",
                    "buffer": PNG_BYTES,
                }
            )
            avatar = dialog.locator('img[alt="头像验收用户的头像"]')
            expect(avatar).to_be_visible()
            avatar_url = avatar.get_attribute("src")
            if not avatar_url or "/api/v1/users/" not in avatar_url:
                raise AssertionError(f"头像地址不正确: {avatar_url}")
            image_response = api.get(avatar_url)
            if image_response.status != 200:
                raise AssertionError(
                    f"公开头像读取失败: {image_response.status}"
                )
            if image_response.headers.get("content-type", "").split(";")[0] != (
                "image/png"
            ):
                raise AssertionError(
                    "公开头像类型错误: "
                    + image_response.headers.get("content-type", "")
                )

            dialog.get_by_label("个人简介").fill("头像保存后仍应保留")
            dialog.get_by_role(
                "button",
                name="保存资料",
                exact=True,
            ).click()
            expect(dialog).not_to_be_visible()
            page.get_by_role("button", name="资料", exact=True).click()
            expect(dialog).to_be_visible()
            expect(
                dialog.locator('img[alt="头像验收用户的头像"]')
            ).to_be_visible()

            screenshot = report_dir / "profile-avatar.png"
            page.screenshot(path=str(screenshot), full_page=True)

            dialog.get_by_role(
                "button",
                name="移除头像",
                exact=True,
            ).click()
            expect(
                dialog.locator('img[alt="头像验收用户的头像"]')
            ).to_have_count(0)
            expect(dialog.locator(".profile-avatar")).to_have_text("头")

            result = {
                "avatar_url": avatar_url,
                "avatar_public_status": image_response.status,
                "avatar_preserved_after_profile_save": True,
                "avatar_removed": True,
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
    parser = argparse.ArgumentParser(description="验证资料头像上传闭环")
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
        default=".tmp/profile-avatar-e2e",
    )
    return parser.parse_args()


if __name__ == "__main__":
    print(json.dumps(run(parse_args()), ensure_ascii=False, indent=2))
