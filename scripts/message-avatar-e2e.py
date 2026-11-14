"""验证私聊和群聊发送者资料、群头像上传与响应式布局"""

from __future__ import annotations

import argparse
import json
import uuid
from pathlib import Path
from typing import Any

from playwright.sync_api import APIRequestContext, Page, expect, sync_playwright

BASE_URL = "http://127.0.0.1:5173"
PASSWORD = "Demo123456!"
PNG_BYTES = (
    b"\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR"
    b"\x00\x00\x00\x01\x00\x00\x00\x01\x08\x06\x00\x00\x00"
    b"\x1f\x15\xc4\x89\x00\x00\x00\rIDAT\x08\xd7c\xf8\xcf\xc0\xf0"
    b"\x1f\x00\x05\x00\x01\xff\x89\x99=\x1d\x00\x00\x00\x00IEND"
    b"\xaeB`\x82"
)


def api_request(
    api: APIRequestContext,
    method: str,
    path: str,
    token: str = "",
    payload: dict[str, Any] | None = None,
    multipart: dict[str, Any] | None = None,
) -> tuple[int, Any]:
    """发送 API 请求并返回状态码与 JSON 响应"""
    headers: dict[str, str] = {}
    data = None
    if token:
        headers["Authorization"] = f"Bearer {token}"
    if payload is not None:
        headers["Content-Type"] = "application/json"
        data = json.dumps(payload, ensure_ascii=False)
    response = api.fetch(
        f"{BASE_URL}/api/v1{path}",
        method=method,
        headers=headers,
        data=data,
        multipart=multipart,
        timeout=15_000,
    )
    body = response.text()
    if not body:
        return response.status, None
    try:
        return response.status, json.loads(body)
    except json.JSONDecodeError:
        return response.status, body


def api_ok(
    api: APIRequestContext,
    method: str,
    path: str,
    token: str = "",
    payload: dict[str, Any] | None = None,
    multipart: dict[str, Any] | None = None,
) -> Any:
    """发送请求并要求接口返回成功状态"""
    status, body = api_request(
        api,
        method,
        path,
        token,
        payload,
        multipart,
    )
    if status < 200 or status >= 300:
        raise RuntimeError(f"{method} {path} 返回 {status}: {body}")
    return body


def create_user(
    api: APIRequestContext,
    suffix: str,
    nickname: str,
) -> dict[str, Any]:
    """注册并登录一个隔离的验收账号"""
    username = f"avatar_{suffix}"
    api_ok(
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
            "device_name": "消息头像验收",
            "platform": 4,
        },
    )
    return {
        "id": str(login["user"]["id"]),
        "username": username,
        "nickname": nickname,
        "token": login["token"],
        "login": login,
    }


def upload_user_avatar(
    api: APIRequestContext,
    user: dict[str, Any],
) -> None:
    """为用户上传一张最小 PNG 头像"""
    api_ok(
        api,
        "POST",
        "/me/avatar",
        token=user["token"],
        multipart={
            "file": {
                "name": "avatar.png",
                "mimeType": "image/png",
                "buffer": PNG_BYTES,
            },
        },
    )


def prepare_friendship(
    api: APIRequestContext,
    requester: dict[str, Any],
    recipient: dict[str, Any],
) -> None:
    """建立两名验收用户之间的好友关系"""
    api_ok(
        api,
        "POST",
        "/friend-requests",
        token=requester["token"],
        payload={
            "target_user_id": recipient["id"],
            "remark": "消息头像验收好友",
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
    """创建群聊、接受邀请并设置一名群管理员"""
    group = api_ok(
        api,
        "POST",
        "/conversations/groups",
        token=owner["token"],
        payload={
            "name": name,
            "member_user_ids": [member["id"]],
            "invite_remark": "消息头像验收群邀请",
        },
    )
    invitations = api_ok(
        api,
        "GET",
        "/group-invitations",
        token=member["token"],
    )
    invitation = next(
        item
        for item in invitations["invitations"]
        if item["conversation_name"] == name
    )
    api_ok(
        api,
        "POST",
        f"/group-invitations/{invitation['invitation_id']}/accept",
        token=member["token"],
    )
    api_ok(
        api,
        "PUT",
        f"/conversations/{group['id']}/members/{member['id']}/role",
        token=owner["token"],
        payload={"role": 2},
    )
    return group


def send_text(
    api: APIRequestContext,
    token: str,
    conversation_id: str,
    content: str,
) -> dict[str, Any]:
    """向指定会话发送一条文本消息"""
    return api_ok(
        api,
        "POST",
        f"/conversations/{conversation_id}/messages",
        token=token,
        payload={
            "client_message_id": str(uuid.uuid4()),
            "message_type": 1,
            "content": content,
        },
    )


def send_file(
    api: APIRequestContext,
    token: str,
    conversation_id: str,
) -> None:
    """上传文本附件并发送文件消息"""
    uploaded = api_ok(
        api,
        "POST",
        "/attachments",
        token=token,
        multipart={
            "file": {
                "name": "layout-check.txt",
                "mimeType": "text/plain",
                "buffer": "消息布局验收文件".encode(),
            },
        },
    )
    api_ok(
        api,
        "POST",
        f"/conversations/{conversation_id}/messages",
        token=token,
        payload={
            "client_message_id": str(uuid.uuid4()),
            "message_type": 3,
            "attachment_id": uploaded["attachment_id"],
        },
    )


def authenticate_page(page: Page, user: dict[str, Any]) -> None:
    """把 API 登录结果写入浏览器并进入聊天页面"""
    page.goto(BASE_URL, wait_until="domcontentloaded")
    page.evaluate(
        """login => {
            localStorage.setItem('relayim.access_token', login.token)
            localStorage.setItem('relayim.refresh_token', login.refresh_token)
            localStorage.setItem('relayim.device_id', login.device_id)
            localStorage.setItem(
                'relayim.user',
                JSON.stringify(login.user),
            )
        }""",
        user["login"],
    )
    page.goto(BASE_URL, wait_until="networkidle")
    expect(page.locator(".rail-connection.connected")).to_be_visible(
        timeout=15_000
    )


def open_conversation(page: Page, title: str) -> None:
    """从左侧会话列表打开指定会话"""
    item = page.locator(".conversation-item").filter(has_text=title).first
    expect(item).to_be_visible(timeout=10_000)
    item.locator(".conversation-open").click()
    expect(page.locator(".message-row").first).to_be_visible(timeout=10_000)


def assert_message_alignment(page: Page, text: str, outgoing: bool) -> None:
    """校验头像位于对方消息左侧、自己消息右侧"""
    row = page.locator(".message-row").filter(has_text=text).first
    expect(row).to_be_visible()
    avatar_box = row.locator(".message-avatar").bounding_box()
    bubble_box = row.locator(".message-bubble").bounding_box()
    scroller_box = page.locator(".message-scroller").bounding_box()
    if not avatar_box or not bubble_box or not scroller_box:
        raise AssertionError(f"消息 {text} 缺少头像或气泡布局")
    if outgoing and bubble_box["x"] >= avatar_box["x"]:
        raise AssertionError("自己的消息头像没有位于气泡右侧")
    if not outgoing and avatar_box["x"] >= bubble_box["x"]:
        raise AssertionError("对方消息头像没有位于气泡左侧")
    avatar_right = avatar_box["x"] + avatar_box["width"]
    scroller_right = scroller_box["x"] + scroller_box["width"]
    if outgoing and avatar_right < scroller_right - 80:
        raise AssertionError("自己的消息没有贴近聊天区最右侧")
    if not outgoing and avatar_box["x"] > scroller_box["x"] + 80:
        raise AssertionError("对方消息没有贴近聊天区最左侧")


def assert_no_horizontal_overflow(page: Page, width: int) -> None:
    """检查指定视口下页面没有横向溢出"""
    page.set_viewport_size({"width": width, "height": 900})
    page.wait_for_timeout(150)
    overflow = page.evaluate(
        "() => document.documentElement.scrollWidth > window.innerWidth"
    )
    if overflow:
        raise AssertionError(f"{width}px 视口出现横向溢出")


def run(output_dir: Path) -> dict[str, Any]:
    """执行完整验收并返回可归档的结果摘要"""
    output_dir.mkdir(parents=True, exist_ok=True)
    suffix = uuid.uuid4().hex[:8]
    group_name = f"消息头像验收群 {suffix}"
    owner_text = f"群主头像消息 {suffix}"
    member_text = f"管理员头像消息 {suffix}"
    direct_owner_text = f"私聊对方消息 {suffix}"
    direct_member_text = f"私聊自己消息 {suffix}"

    with sync_playwright() as playwright:
        browser = playwright.chromium.launch(headless=True)
        owner_context = browser.new_context(viewport={"width": 1440, "height": 960})
        member_context = browser.new_context(viewport={"width": 1440, "height": 960})
        owner_page = owner_context.new_page()
        member_page = member_context.new_page()
        page_errors: list[str] = []
        for page in (owner_page, member_page):
            page.on("pageerror", lambda error: page_errors.append(str(error)))

        api = owner_page.request
        owner = create_user(api, f"owner_{suffix}", f"群主验收{suffix}")
        member = create_user(api, f"member_{suffix}", f"管理员验收{suffix}")
        upload_user_avatar(api, owner)
        upload_user_avatar(api, member)
        prepare_friendship(api, owner, member)

        direct = api_ok(
            api,
            "POST",
            "/conversations/private",
            token=owner["token"],
            payload={"peer_user_id": member["id"]},
        )
        direct_member_view = api_ok(
            api,
            "GET",
            f"/conversations/{direct['id']}",
            token=member["token"],
        )
        direct_title = direct_member_view["title"]
        send_text(api, owner["token"], direct["id"], direct_owner_text)
        send_text(api, member["token"], direct["id"], direct_member_text)

        group = prepare_group(api, owner, member, group_name)
        forbidden_status, _ = api_request(
            api,
            "POST",
            f"/conversations/{group['id']}/avatar",
            token=member["token"],
            multipart={
                "file": {
                    "name": "forbidden.png",
                    "mimeType": "image/png",
                    "buffer": PNG_BYTES,
                },
            },
        )
        if forbidden_status != 403:
            raise AssertionError(
                f"管理员上传群头像应返回 403，实际为 {forbidden_status}"
            )
        send_text(api, owner["token"], group["id"], owner_text)
        send_text(api, member["token"], group["id"], member_text)
        recalled = send_text(
            api,
            owner["token"],
            group["id"],
            f"待撤回消息 {suffix}",
        )
        api_ok(
            api,
            "DELETE",
            f"/conversations/{group['id']}/messages/{recalled['message_id']}",
            token=owner["token"],
        )
        send_file(api, owner["token"], group["id"])

        authenticate_page(owner_page, owner)
        authenticate_page(member_page, member)

        open_conversation(member_page, group_name)
        expect(
            member_page.locator(".message-row").filter(has_text=owner_text)
        ).to_contain_text(owner["nickname"])
        expect(
            member_page.locator(".message-row").filter(has_text=member_text)
        ).to_contain_text(member["nickname"])
        expect(
            member_page.locator(".message-row").filter(has_text=owner_text)
        ).to_contain_text("群主")
        expect(
            member_page.locator(".message-row").filter(has_text=member_text)
        ).to_contain_text("管理员")
        assert_message_alignment(member_page, owner_text, outgoing=False)
        assert_message_alignment(member_page, member_text, outgoing=True)
        expect(
            member_page.locator(".message-row").filter(
                has_text=owner_text
            ).locator(".message-avatar img")
        ).to_be_visible()
        expect(
            member_page.locator(".message-row").filter(
                has_text=member_text
            ).locator(".message-avatar img")
        ).to_be_visible()
        expect(member_page.locator(".message-file").first).to_be_visible()
        expect(member_page.get_by_text("该消息已撤回").first).to_be_visible()
        member_page.screenshot(
            path=output_dir / "group-messages.png",
            full_page=True,
        )

        open_conversation(member_page, direct_title)
        expect(
            member_page.locator(".message-row").filter(
                has_text=direct_owner_text
            )
        ).to_contain_text(owner["nickname"])
        expect(
            member_page.locator(".message-row").filter(
                has_text=direct_member_text
            )
        ).to_contain_text(member["nickname"])
        assert_message_alignment(
            member_page,
            direct_owner_text,
            outgoing=False,
        )
        assert_message_alignment(
            member_page,
            direct_member_text,
            outgoing=True,
        )
        member_page.screenshot(
            path=output_dir / "direct-messages.png",
            full_page=True,
        )

        open_conversation(owner_page, group_name)
        owner_page.get_by_role(
            "button",
            name="群成员列表",
            exact=True,
        ).click()
        dialog = owner_page.get_by_role("dialog", name="群成员")
        expect(dialog).to_be_visible()
        file_input = dialog.locator('input[type="file"]')
        file_input.set_input_files(
            {
                "name": "group-avatar.png",
                "mimeType": "image/png",
                "buffer": PNG_BYTES,
            }
        )
        preview = dialog.locator(".group-avatar-preview img")
        expect(preview).to_be_visible(timeout=10_000)
        avatar_url = preview.get_attribute("src")
        if not avatar_url or "/api/v1/conversations/" not in avatar_url:
            raise AssertionError(f"群头像地址不正确: {avatar_url}")
        image_response = owner_page.request.get(f"{BASE_URL}{avatar_url}")
        if image_response.status != 200:
            raise AssertionError(f"群头像公开读取返回 {image_response.status}")
        owner_page.screenshot(
            path=output_dir / "group-avatar-upload.png",
            full_page=True,
        )

        dialog.get_by_role("button", name="移除头像").click()
        expect(dialog.locator(".group-avatar-preview img")).to_have_count(0)
        file_input.set_input_files(
            {
                "name": "group-avatar.png",
                "mimeType": "image/png",
                "buffer": PNG_BYTES,
            }
        )
        expect(dialog.locator(".group-avatar-preview img")).to_be_visible(
            timeout=10_000
        )
        dialog.get_by_role("button", name="关闭群成员").click()
        expect(
            owner_page.locator(".conversation-item").filter(
                has_text=group_name
            ).locator(".conversation-avatar img")
        ).to_be_visible()

        open_conversation(member_page, group_name)
        member_page.get_by_role(
            "button",
            name="群成员列表",
            exact=True,
        ).click()
        member_dialog = member_page.get_by_role("dialog", name="群成员")
        expect(member_dialog).to_be_visible()
        expect(member_dialog.locator(".group-profile-form")).to_have_count(0)
        member_dialog.get_by_role("button", name="关闭群成员").click()

        for width in (390, 320):
            assert_no_horizontal_overflow(member_page, width)
            member_page.screenshot(
                path=output_dir / f"group-{width}.png",
                full_page=True,
            )
        assert_no_horizontal_overflow(owner_page, 390)

        if page_errors:
            raise AssertionError(f"页面运行错误: {page_errors}")

        browser.close()
        return {
            "group_id": group["id"],
            "group_avatar_url": avatar_url,
            "owner": owner["username"],
            "member": member["username"],
            "viewport_checks": [320, 390, 1440],
            "screenshots": [
                str(output_dir / "group-messages.png"),
                str(output_dir / "direct-messages.png"),
                str(output_dir / "group-avatar-upload.png"),
                str(output_dir / "group-320.png"),
                str(output_dir / "group-390.png"),
            ],
        }


def main() -> None:
    """解析输出目录并运行验收"""
    parser = argparse.ArgumentParser()
    parser.add_argument(
        "--output",
        type=Path,
        default=Path(".tmp/message-avatar-e2e"),
    )
    args = parser.parse_args()
    result = run(args.output.resolve())
    print(json.dumps(result, ensure_ascii=False, indent=2))


if __name__ == "__main__":
    main()
