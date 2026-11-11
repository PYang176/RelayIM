"""验证认证、好友、消息和分页接口的异常与边界行为。"""

from __future__ import annotations

import argparse
import json
import os
import time
from datetime import UTC, datetime
from pathlib import Path
from typing import Any

from playwright.sync_api import APIRequestContext, sync_playwright

PASSWORD = "Boundary123456!"


def api_request(
    api: APIRequestContext,
    method: str,
    path: str,
    token: str = "",
    payload: dict[str, Any] | None = None,
) -> tuple[int, Any]:
    """发送 JSON API 请求并返回状态码与解析后的响应。"""
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
    content = response.text()
    return response.status, json.loads(content) if content else None


def expect_status(
    api: APIRequestContext,
    method: str,
    path: str,
    expected: int,
    token: str = "",
    payload: dict[str, Any] | None = None,
) -> Any:
    """断言接口状态码符合预期并返回响应正文。"""
    status, body = api_request(api, method, path, token, payload)
    if status != expected:
        raise AssertionError(f"{method} {path} = {status}，期望 {expected}: {body}")
    return body


def create_session(
    api: APIRequestContext,
    username: str,
    nickname: str,
) -> dict[str, Any]:
    """注册并登录一个边界测试账号。"""
    user = expect_status(
        api,
        "POST",
        "/auth/register",
        201,
        payload={
            "username": username,
            "password": PASSWORD,
            "confirm_password": PASSWORD,
            "nickname": nickname,
        },
    )
    login = expect_status(
        api,
        "POST",
        "/auth/login",
        200,
        payload={
            "username": username,
            "password": PASSWORD,
            "device_name": "HTTP 边界验收",
            "platform": 4,
        },
    )
    return {
        "id": str(user["id"]),
        "username": username,
        "nickname": nickname,
        "token": login["token"],
    }


def require_relationship(
    api: APIRequestContext,
    user: dict[str, Any],
    target_user_id: str,
    expected: dict[str, bool],
) -> dict[str, Any]:
    """验证关系状态响应中的动作字段。"""
    body = expect_status(
        api,
        "GET",
        f"/friends/{target_user_id}/relationship",
        200,
        token=user["token"],
    )
    for field, value in expected.items():
        if body.get(field) is not value:
            raise AssertionError(f"关系字段 {field} = {body.get(field)}，期望 {value}")
    return body


def run(args: argparse.Namespace) -> dict[str, Any]:
    """执行完整边界测试并返回可安全公开的结果摘要。"""
    suffix = (
        datetime.now(UTC).strftime("%m%d%H%M%S")
        + f"{time.time_ns() % 1000:03d}"
    )
    report_dir = Path(args.output_dir)
    report_dir.mkdir(parents=True, exist_ok=True)

    with sync_playwright() as playwright:
        api = playwright.request.new_context(base_url=args.api_url)
        try:
            alice = create_session(
                api,
                f"boundary_a_{suffix}",
                f"边界用户甲{suffix}",
            )
            bob = create_session(
                api,
                f"boundary_b_{suffix}",
                f"边界用户乙{suffix}",
            )

            expect_status(
                api,
                "GET",
                f"/friends/{bob['id']}/relationship",
                401,
            )
            expect_status(
                api,
                "GET",
                "/friends/not-a-number/relationship",
                400,
                token=alice["token"],
            )
            expect_status(
                api,
                "GET",
                f"/friends/{alice['id']}/relationship",
                400,
                token=alice["token"],
            )
            expect_status(
                api,
                "GET",
                "/friends/1/relationship",
                404,
                token=alice["token"],
            )

            for remark, expected in ((" ", 400), ("边" * 201, 400), ("边" * 200, 201)):
                expect_status(
                    api,
                    "POST",
                    "/friend-requests",
                    expected,
                    token=alice["token"],
                    payload={
                        "target_user_id": bob["id"],
                        "remark": remark,
                    },
                )

            expect_status(
                api,
                "POST",
                "/friend-requests",
                409,
                token=alice["token"],
                payload={
                    "target_user_id": bob["id"],
                    "remark": "重复申请",
                },
            )
            require_relationship(
                api,
                alice,
                bob["id"],
                {
                    "can_message": False,
                    "can_request": False,
                    "has_pending_request": True,
                    "requested_by_me": True,
                },
            )
            require_relationship(
                api,
                bob,
                alice["id"],
                {
                    "can_message": False,
                    "can_request": False,
                    "has_pending_request": True,
                    "requested_by_me": False,
                },
            )

            received = expect_status(
                api,
                "GET",
                "/friend-requests/received",
                200,
                token=bob["token"],
            )
            request = next(
                item
                for item in received["requests"]
                if item["user"]["id"] == alice["id"]
            )
            expect_status(
                api,
                "POST",
                f"/friend-requests/{request['id']}/accept",
                200,
                token=bob["token"],
            )
            require_relationship(
                api,
                alice,
                bob["id"],
                {
                    "can_message": True,
                    "can_request": False,
                    "has_pending_request": False,
                },
            )

            expect_status(
                api,
                "PUT",
                f"/friends/{bob['id']}/remark",
                200,
                token=alice["token"],
                payload={"remark": "备" * 64},
            )
            expect_status(
                api,
                "PUT",
                f"/friends/{bob['id']}/remark",
                400,
                token=alice["token"],
                payload={"remark": "备" * 65},
            )

            conversation = expect_status(
                api,
                "POST",
                "/conversations/private",
                201,
                token=alice["token"],
                payload={"peer_user_id": bob["id"]},
            )
            conversation_id = conversation["id"]

            expect_status(
                api,
                "GET",
                f"/conversations/{conversation_id}/messages?limit=101",
                400,
                token=alice["token"],
            )
            expect_status(
                api,
                "GET",
                "/friends?limit=101",
                400,
                token=alice["token"],
            )
            expect_status(
                api,
                "GET",
                "/users/search?q=",
                400,
                token=alice["token"],
            )
            expect_status(
                api,
                "GET",
                f"/users/search?q={'搜' * 65}",
                400,
                token=alice["token"],
            )

            for client_message_id, content, expected in (
                ("empty-content", " ", 400),
                ("content-too-long", "消" * 8001, 400),
                ("boundary-content", "消" * 8000, 201),
            ):
                expect_status(
                    api,
                    "POST",
                    f"/conversations/{conversation_id}/messages",
                    expected,
                    token=alice["token"],
                    payload={
                        "client_message_id": client_message_id,
                        "content": content,
                    },
                )

            first = expect_status(
                api,
                "POST",
                f"/conversations/{conversation_id}/messages",
                201,
                token=alice["token"],
                payload={
                    "client_message_id": f"idempotent-{suffix}",
                    "content": "幂等边界消息",
                },
            )
            duplicate = expect_status(
                api,
                "POST",
                f"/conversations/{conversation_id}/messages",
                200,
                token=alice["token"],
                payload={
                    "client_message_id": f"idempotent-{suffix}",
                    "content": "幂等边界消息",
                },
            )
            if not duplicate["duplicate"] or duplicate["seq"] != first["seq"]:
                raise AssertionError("重复客户端消息 ID 未返回同一服务端消息")

            expect_status(
                api,
                "POST",
                f"/users/{bob['id']}/block",
                200,
                token=alice["token"],
            )
            require_relationship(
                api,
                alice,
                bob["id"],
                {
                    "can_message": False,
                    "can_request": False,
                    "blocked_by_me": True,
                },
            )
            expect_status(
                api,
                "POST",
                f"/conversations/{conversation_id}/messages",
                403,
                token=bob["token"],
                payload={
                    "client_message_id": f"blocked-{suffix}",
                    "content": "拉黑后消息",
                },
            )
            expect_status(
                api,
                "DELETE",
                f"/users/{bob['id']}/block",
                200,
                token=alice["token"],
            )
            expect_status(
                api,
                "DELETE",
                f"/friends/{bob['id']}",
                200,
                token=alice["token"],
            )
            require_relationship(
                api,
                bob,
                alice["id"],
                {
                    "can_message": False,
                    "can_request": True,
                    "has_pending_request": False,
                },
            )
            expect_status(
                api,
                "POST",
                "/friend-requests",
                201,
                token=bob["token"],
                payload={
                    "target_user_id": alice["id"],
                    "remark": "被删除方重新申请",
                },
            )

            result = {
                "authentication_boundaries": True,
                "relationship_boundaries": True,
                "friend_request_boundaries": True,
                "message_boundaries": True,
                "pagination_boundaries": True,
                "block_boundaries": True,
                "unilateral_delete_reapply": True,
            }
            (report_dir / "result.json").write_text(
                json.dumps(result, ensure_ascii=False, indent=2),
                encoding="utf-8",
            )
            return result
        finally:
            api.dispose()


def parse_args() -> argparse.Namespace:
    """读取边界测试命令行参数。"""
    parser = argparse.ArgumentParser(description="验证 RelayIM HTTP 异常和边界")
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
            ".tmp/http-boundary-e2e",
        ),
    )
    return parser.parse_args()


if __name__ == "__main__":
    print(json.dumps(run(parse_args()), ensure_ascii=False, indent=2))
