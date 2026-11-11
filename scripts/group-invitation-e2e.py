"""验证群邀请的浏览器同意、拒绝和入群结果。"""

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
        timeout=15_000,
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


def ensure_user(
    api: APIRequestContext,
    username: str,
    nickname: str,
) -> dict[str, Any]:
    status, registered = api_request(
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
    status, login = api_request(
        api,
        "POST",
        "/auth/login",
        payload={
            "username": username,
            "password": PASSWORD,
            "device_name": "群邀请验收",
            "platform": 4,
        },
    )
    if status != 200:
        raise RuntimeError(f"群邀请验收账号登录失败: {status} {login}")
    user_id = (
        str(registered["id"])
        if registered and registered.get("id")
        else str(login["user"]["id"])
    )
    return {
        "id": user_id,
        "username": username,
        "nickname": nickname,
        "token": login["token"],
    }


def make_friends(
    api: APIRequestContext,
    owner: dict[str, Any],
    invitee: dict[str, Any],
) -> None:
    friends = api_ok(
        api,
        "GET",
        "/friends?limit=100",
        token=owner["token"],
    )
    if any(
        str(item["user"]["id"]) == invitee["id"]
        for item in friends["friends"]
    ):
        return
    received = api_ok(
        api,
        "GET",
        "/friend-requests/received",
        token=invitee["token"],
    )
    existing = next(
        (
            item
            for item in received["requests"]
            if str(item["user"]["id"]) == owner["id"]
        ),
        None,
    )
    if existing is None:
        api_ok(
            api,
            "POST",
            "/friend-requests",
            token=owner["token"],
            payload={
                "target_user_id": invitee["id"],
                "remark": "群邀请验收好友",
            },
        )
        received = api_ok(
            api,
            "GET",
            "/friend-requests/received",
            token=invitee["token"],
        )
        existing = next(
            item
            for item in received["requests"]
            if str(item["user"]["id"]) == owner["id"]
        )
    if existing:
        api_ok(
            api,
            "POST",
            f"/friend-requests/{existing['id']}/accept",
            token=invitee["token"],
        )


def create_group_with_invitation(
    api: APIRequestContext,
    owner: dict[str, Any],
    invitee: dict[str, Any],
    name: str,
) -> str:
    group = api_ok(
        api,
        "POST",
        "/conversations/groups",
        token=owner["token"],
        payload={
            "name": name,
            "avatar_url": "",
            "member_user_ids": [invitee["id"]],
            "invite_remark": f"邀请加入 {name}",
        },
    )
    return group["id"]


def list_received(
    api: APIRequestContext,
    invitee: dict[str, Any],
) -> list[dict[str, Any]]:
    result = api_ok(
        api,
        "GET",
        "/group-invitations?limit=20",
        token=invitee["token"],
    )
    return result["invitations"]


def list_group_history(
    api: APIRequestContext,
    owner: dict[str, Any],
    conversation_id: str,
) -> list[dict[str, Any]]:
    result = api_ok(
        api,
        "GET",
        f"/conversations/{conversation_id}/invitations?limit=20",
        token=owner["token"],
    )
    return result["invitations"]


def run(args: argparse.Namespace) -> dict[str, Any]:
    suffix = (
        datetime.now(UTC).strftime("%m%d%H%M%S")
        + f"{time.time_ns() % 1000:03d}"
    )
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
            owner = ensure_user(
                api,
                "group_invite_owner_e2e",
                "群邀请验收群主",
            )
            invitee = ensure_user(
                api,
                "group_invite_invitee_e2e",
                "群邀请验收成员",
            )
            make_friends(api, owner, invitee)

            for pending in list_received(api, invitee):
                api_ok(
                    api,
                    "POST",
                    f"/group-invitations/{pending['invitation_id']}/reject",
                    token=invitee["token"],
                    payload={"response_remark": "清理历史验收邀请"},
                )

            page.goto("/login", wait_until="networkidle")
            page.get_by_label("用户名").fill(invitee["username"])
            page.get_by_label("密码").fill(PASSWORD)
            page.get_by_role("button", name="进入 RelayIM").click()
            page.wait_for_url("**/")
            invitation_button = page.get_by_role(
                "button",
                name="群邀请",
                exact=True,
            )
            expect(invitation_button).to_contain_text("群邀请", timeout=10_000)

            accepted_group_name = f"邀请同意群 {suffix}"
            accepted_notification_started = time.monotonic()
            accepted_group_id = create_group_with_invitation(
                api,
                owner,
                invitee,
                accepted_group_name,
            )
            invitations = list_received(api, invitee)
            accepted_invitation = next(
                item
                for item in invitations
                if item["conversation_id"] == accepted_group_id
            )
            accepted_remark = f"邀请加入 {accepted_group_name}"
            if accepted_invitation["invite_remark"] != accepted_remark:
                raise AssertionError("群邀请备注与提交内容不一致")
            expect(
                invitation_button.locator(".rail-badge")
            ).to_have_text("1", timeout=5000)
            accepted_notification_ms = round(
                (time.monotonic() - accepted_notification_started) * 1000
            )
            invitation_button.click()

            dialog = page.locator(".friends-dialog").filter(
                has_text="群邀请"
            )
            invitation_row = dialog.locator(".request-row").filter(
                has_text=accepted_group_name
            )
            expect(invitation_row).to_be_visible()
            expect(
                invitation_row.get_by_text(accepted_remark, exact=True)
            ).to_be_visible()
            accept_response = "同意加入，稍后查看群消息"
            invitation_row.get_by_label("审批回复备注").fill(accept_response)
            dialog_screenshot = report_dir / "group-invitation-pending.png"
            page.screenshot(path=str(dialog_screenshot), full_page=True)
            invitation_row.get_by_role(
                "button",
                name="同意",
                exact=True,
            ).click()

            page.get_by_role("heading", name=accepted_group_name).wait_for(
                timeout=15_000
            )
            accepted_conversations = api_ok(
                api,
                "GET",
                "/conversations",
                token=invitee["token"],
            )
            if not any(
                item["id"] == accepted_group_id
                for item in accepted_conversations["conversations"]
            ):
                raise AssertionError("接受邀请后会话列表缺少目标群")
            accepted_history = list_group_history(
                api,
                owner,
                accepted_group_id,
            )
            accepted_record = next(
                item
                for item in accepted_history
                if item["invitation_id"]
                == accepted_invitation["invitation_id"]
            )
            if (
                accepted_record["status"] != 2
                or accepted_record["response_remark"] != accept_response
            ):
                raise AssertionError("接受邀请后的审批记录不正确")
            accepted_screenshot = report_dir / "group-invitation-accepted.png"
            page.screenshot(path=str(accepted_screenshot), full_page=True)

            rejected_group_name = f"邀请拒绝群 {suffix}"
            rejected_group_id = create_group_with_invitation(
                api,
                owner,
                invitee,
                rejected_group_name,
            )
            expect(
                invitation_button.locator(".rail-badge")
            ).to_have_text("1", timeout=5000)
            invitation_button.click()
            dialog = page.locator(".friends-dialog").filter(
                has_text="群邀请"
            )
            rejected_row = dialog.locator(".request-row").filter(
                has_text=rejected_group_name
            )
            expect(rejected_row).to_be_visible()
            reject_response = "最近较忙，暂时不参加"
            rejected_row.get_by_label("审批回复备注").fill(reject_response)
            rejected_row.get_by_role(
                "button",
                name="拒绝",
                exact=True,
            ).click()
            expect(rejected_row).to_have_count(0, timeout=10_000)

            final_conversations = api_ok(
                api,
                "GET",
                "/conversations",
                token=invitee["token"],
            )
            if any(
                item["id"] == rejected_group_id
                for item in final_conversations["conversations"]
            ):
                raise AssertionError("拒绝邀请后仍然出现在群会话列表")
            rejected_history = list_group_history(
                api,
                owner,
                rejected_group_id,
            )
            rejected_record = rejected_history[0]
            if (
                rejected_record["status"] != 3
                or rejected_record["response_remark"] != reject_response
            ):
                raise AssertionError("拒绝邀请后的审批记录不正确")

            canceled_group_name = f"邀请取消群 {suffix}"
            canceled_group_id = create_group_with_invitation(
                api,
                owner,
                invitee,
                canceled_group_name,
            )
            expect(
                invitation_button.locator(".rail-badge")
            ).to_have_text("1", timeout=5000)
            canceled_invitation = next(
                item
                for item in list_received(api, invitee)
                if item["conversation_id"] == canceled_group_id
            )
            cancellation_notification_started = time.monotonic()
            api_ok(
                api,
                "DELETE",
                "/conversations/"
                f"{canceled_group_id}/invitations/"
                f"{canceled_invitation['invitation_id']}",
                token=owner["token"],
            )
            expect(invitation_button).to_contain_text("群邀请", timeout=5000)
            expect(invitation_button.locator(".rail-badge")).to_have_count(0)
            cancellation_notification_ms = round(
                (time.monotonic() - cancellation_notification_started) * 1000
            )
            canceled_history = list_group_history(
                api,
                owner,
                canceled_group_id,
            )
            if canceled_history[0]["status"] != 5:
                raise AssertionError("取消邀请后的状态不是已取消")
            if any(
                item["conversation_id"] == canceled_group_id
                for item in list_received(api, invitee)
            ):
                raise AssertionError("取消后邀请仍在接收箱")

            owner_page = browser.new_page(base_url=args.frontend_url)
            owner_page.goto("/login", wait_until="networkidle")
            owner_page.get_by_label("用户名").fill(owner["username"])
            owner_page.get_by_label("密码").fill(PASSWORD)
            owner_page.get_by_role("button", name="进入 RelayIM").click()
            owner_page.wait_for_url("**/")
            owner_page.locator(".conversation-item").filter(
                has_text=rejected_group_name
            ).click()
            owner_page.get_by_role(
                "button",
                name="群成员列表",
                exact=True,
            ).click()
            member_dialog = owner_page.locator(".group-members-dialog")
            expect(member_dialog).to_be_visible()
            expect(
                member_dialog.get_by_text("群邀请记录", exact=True)
            ).to_be_visible()
            expect(
                member_dialog.get_by_text(reject_response, exact=False)
            ).to_be_visible()
            history_screenshot = report_dir / "group-invitation-history.png"
            owner_page.screenshot(
                path=str(history_screenshot),
                full_page=True,
            )

            result = {
                "accepted_invitation_id": accepted_invitation["invitation_id"],
                "accepted_group_id": accepted_group_id,
                "rejected_group_id": rejected_group_id,
                "remaining_invitations": len(list_received(api, invitee)),
                "pending_screenshot": str(dialog_screenshot),
                "accepted_screenshot": str(accepted_screenshot),
                "history_screenshot": str(history_screenshot),
                "canceled_invitation_id": canceled_invitation["invitation_id"],
                "invitation_realtime_ms": accepted_notification_ms,
                "cancellation_realtime_ms": cancellation_notification_ms,
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
    parser = argparse.ArgumentParser(description="验证群邀请审批链路")
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
            ".tmp/group-invitation-e2e",
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
