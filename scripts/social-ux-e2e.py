"""验证联系人资料工作区、资料页管理操作和单向删除关系。"""

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


def prepare_group(
    api: APIRequestContext,
    owner: dict[str, Any],
    member: dict[str, Any],
    name: str,
) -> dict[str, Any]:
    return api_ok(
        api,
        "POST",
        "/conversations/groups",
        token=owner["token"],
        payload={
            "name": name,
            "member_user_ids": [member["id"]],
            "invite_remark": "联系人工作区验收群",
        },
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
    page.get_by_role("button", name="联系人", exact=True).click()
    panel = page.locator(".contacts-panel")
    expect(panel).to_be_visible()
    return panel


def run(args: argparse.Namespace) -> dict[str, Any]:
    suffix = (
        datetime.now(UTC).strftime("%m%d%H%M%S")
        + f"{time.time_ns() % 1000:03d}"
    )
    alice_name = f"ux_a_{suffix}"
    bob_name = f"ux_b_{suffix}"
    carol_name = f"ux_c_{suffix}"
    dave_name = f"ux_d_{suffix}"
    alice_nickname = f"体验阿岚{suffix}"
    bob_nickname = f"体验北辰{suffix}"
    carol_nickname = f"体验小禾{suffix}"
    dave_nickname = f"体验远山{suffix}"
    group_name = f"联系人工作区群{suffix}"
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
            carol = create_user(api, carol_name, carol_nickname)
            create_user(api, dave_name, dave_nickname)
            prepare_friendship(api, alice, bob)
            prepare_friendship(api, alice, carol)
            prepare_group(api, alice, carol, group_name)

            login_ui(alice_page, alice_name)
            login_ui(bob_page, bob_name)

            contacts = open_contacts(alice_page)
            expect(
                contacts.get_by_role("button", name="添加好友", exact=True)
            ).to_be_visible()
            expect(alice_page.locator(".contact-placeholder")).to_be_visible()

            contacts.get_by_role("button", name="添加好友", exact=True).click()
            expect(contacts.locator("#add-friend-title")).to_be_visible()
            contacts.get_by_label("搜索用户名或昵称").fill(dave_name)
            contacts.get_by_role("button", name="搜索", exact=True).click()
            expect(
                contacts.locator(".search-result-row").filter(
                    has_text=dave_nickname
                )
            ).to_be_visible()
            contacts.get_by_role("button", name="返回", exact=True).click()

            friend_row = contacts.locator(".friend-row").filter(
                has_text=bob_nickname
            )
            expect(friend_row).to_be_visible()
            expect(friend_row).not_to_contain_text("发消息")
            expect(friend_row).not_to_contain_text("备注")
            expect(friend_row).not_to_contain_text("拉黑")
            expect(friend_row).not_to_contain_text("删除")
            friend_row.locator(".friend-avatar").click()
            detail = alice_page.locator(".contact-detail-panel")
            expect(detail).to_contain_text(bob_nickname)
            expect(
                detail.get_by_role("button", name="发私聊", exact=True)
            ).to_be_visible()
            expect(
                detail.get_by_role("button", name="添加到群聊", exact=True)
            ).to_be_enabled()

            detail.get_by_role(
                "button",
                name="修改备注",
                exact=True,
            ).click()
            detail.get_by_label("好友备注").fill("北辰联系人")
            detail.get_by_role("button", name="保存", exact=True).click()
            expect(friend_row).to_contain_text("北辰联系人", timeout=10_000)

            detail.get_by_role(
                "button",
                name="修改备注",
                exact=True,
            ).click()
            detail.get_by_label("好友备注").fill("")
            detail.get_by_role("button", name="保存", exact=True).click()
            expect(friend_row).to_contain_text(bob_nickname, timeout=10_000)

            detail.get_by_role("button", name="拉黑", exact=True).click()
            expect(detail).to_contain_text("确认拉黑？")
            detail.get_by_role(
                "button",
                name="确认拉黑",
                exact=True,
            ).click()
            expect(detail.locator(".contact-relationship-state")).to_have_text(
                "已拉黑",
                timeout=10_000,
            )
            expect(friend_row).to_contain_text("已拉黑", timeout=10_000)
            detail.get_by_role(
                "button",
                name="取消拉黑",
                exact=True,
            ).click()
            expect(detail.locator(".contact-relationship-state")).to_have_text(
                "好友关系正常",
                timeout=10_000,
            )

            detail.locator(".contact-profile-avatar").click()
            avatar_preview = alice_page.get_by_role("dialog", name="头像预览")
            expect(avatar_preview).to_be_visible()
            avatar_preview.get_by_role(
                "button",
                name="关闭头像预览",
            ).click()

            detail.get_by_role(
                "button",
                name="添加到群聊",
                exact=True,
            ).click()
            group_dialog = alice_page.get_by_role(
                "dialog",
                name="添加到群聊",
            )
            expect(group_dialog).to_be_visible()
            group_dialog.get_by_label("邀请备注").fill("邀请加入联系人验收群")
            group_dialog.get_by_role(
                "button",
                name="发送邀请",
                exact=True,
            ).click()
            expect(group_dialog).not_to_be_visible(timeout=10_000)
            invitations = api_ok(
                api,
                "GET",
                "/group-invitations",
                token=bob["token"],
            )
            if not any(
                item["conversation_name"] == group_name
                for item in invitations["invitations"]
            ):
                raise AssertionError("联系人加入群聊后未生成群邀请")

            contacts_screenshot = report_dir / "contacts-workspace.png"
            alice_page.screenshot(path=str(contacts_screenshot), full_page=True)

            friend_row.locator(".friend-copy").dblclick()
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

            bob_page.get_by_role(
                "button",
                name="聊天记录",
                exact=True,
            ).click()
            history_dialog = bob_page.get_by_role(
                "dialog",
                name="聊天记录",
            )
            expect(history_dialog).to_be_visible()
            history_dialog.get_by_label("搜索聊天记录").fill("单")
            history_dialog.get_by_role(
                "button",
                name="搜索",
                exact=True,
            ).click()
            expect(
                history_dialog.locator(".message-history-entry")
            ).to_contain_text("单向好友体验消息", timeout=10_000)
            history_dialog.get_by_role(
                "button",
                name="关闭聊天记录",
            ).click()

            bob_contacts = open_contacts(bob_page)
            bob_friend_row = bob_contacts.locator(".friend-row").filter(
                has_text=alice_nickname
            )
            expect(bob_friend_row).to_be_visible()

            alice_contacts = open_contacts(alice_page)
            alice_friend_row = alice_contacts.locator(".friend-row").filter(
                has_text=bob_nickname
            )
            alice_friend_row.locator(".friend-avatar").click()
            detail.get_by_role(
                "button",
                name="删除好友",
                exact=True,
            ).click()
            expect(detail).to_contain_text("确认删除好友？")
            detail.get_by_role(
                "button",
                name="确认删除",
                exact=True,
            ).click()
            expect(
                alice_contacts.locator(".friend-row").filter(
                    has_text=bob_nickname
                )
            ).to_have_count(0, timeout=10_000)
            expect(alice_page.locator(".contact-placeholder")).to_be_visible()

            expect(bob_friend_row).to_contain_text(
                "双方当前不是好友",
                timeout=10_000,
            )
            bob_page.get_by_role(
                "button",
                name="消息",
                exact=True,
            ).click()
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
            mobile_contacts = open_contacts(mobile_page)
            mobile_contacts.locator(".friend-row").filter(
                has_text=carol_nickname
            ).locator(".friend-avatar").click()
            expect(mobile_page.locator(".contact-detail-panel")).to_contain_text(
                carol_nickname
            )
            mobile_screenshot = report_dir / "social-ux-mobile.png"
            mobile_page.screenshot(path=str(mobile_screenshot), full_page=True)
            result = {
                "alice_id": alice["id"],
                "bob_id": bob["id"],
                "conversation_visible_without_refresh": True,
                "contacts_workspace": True,
                "avatar_preview": True,
                "single_character_search": True,
                "group_invitation_created": True,
                "double_click_direct": True,
                "contact_actions_in_detail": True,
                "unilateral_delete_status": status,
                "contacts_screenshot": str(contacts_screenshot),
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
