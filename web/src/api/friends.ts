import {request} from './http'
import type {
    BlockedUserPage,
    FriendPage,
    FriendRelationshipStatus,
    FriendRequestPage,
    FriendUser,
} from '@/types/api'

// pageParams 构造好友相关分页查询参数
function pageParams(limit = 20, beforeID = ''): URLSearchParams {
    const params = new URLSearchParams({limit: String(limit)})
    if (beforeID) params.set('before_id', beforeID)
    return params
}

// searchUsers 按用户名或昵称搜索可添加的用户
export async function searchUsers(
    token: string,
    query: string,
): Promise<FriendUser[]> {
    const params = new URLSearchParams({q: query})
    const response = await request<{ users: FriendUser[] }>(
        `/users/search?${params}`,
        {},
        token,
    )
    return response.users
}

// sendFriendRequest 发送带申请理由的好友申请
export function sendFriendRequest(
    token: string,
    targetUserID: string,
    remark: string,
): Promise<void> {
    return request(
        '/friend-requests',
        {
            method: 'POST',
            body: JSON.stringify({
                target_user_id: targetUserID,
                remark,
            }),
        },
        token,
    )
}

// listReceivedRequests 分页查询收到的好友申请
export async function listReceivedRequests(
    token: string,
    beforeID = '',
): Promise<FriendRequestPage> {
    const params = pageParams(20, beforeID)
    return request<FriendRequestPage>(
        `/friend-requests/received?${params}`,
        {},
        token,
    )
}

// listSentRequests 分页查询已发出的好友申请
export async function listSentRequests(
    token: string,
    beforeID = '',
): Promise<FriendRequestPage> {
    const params = pageParams(20, beforeID)
    return request<FriendRequestPage>(
        `/friend-requests/sent?${params}`,
        {},
        token,
    )
}

// acceptFriendRequest 接受好友申请并建立好友关系
export function acceptFriendRequest(token: string, requestID: string): Promise<void> {
    return request(
        `/friend-requests/${requestID}/accept`,
        {method: 'POST'},
        token,
    )
}

// rejectFriendRequest 拒绝好友申请
export function rejectFriendRequest(token: string, requestID: string): Promise<void> {
    return request(
        `/friend-requests/${requestID}/reject`,
        {method: 'POST'},
        token,
    )
}

// listFriends 分页查询好友列表
export async function listFriends(
    token: string,
    beforeID = '',
): Promise<FriendPage> {
    const params = pageParams(20, beforeID)
    return request<FriendPage>(`/friends?${params}`, {}, token)
}

// getRelationshipStatus 查询当前用户与目标用户可以执行的社交动作
export function getRelationshipStatus(
    token: string,
    targetUserID: string,
): Promise<FriendRelationshipStatus> {
    return request(`/friends/${targetUserID}/relationship`, {}, token)
}

// setFriendRemark 更新指定好友的备注
export function setFriendRemark(
    token: string,
    friendUserID: string,
    remark: string,
): Promise<void> {
    return request(
        `/friends/${friendUserID}/remark`,
        {
            method: 'PUT',
            body: JSON.stringify({remark}),
        },
        token,
    )
}

// deleteFriend 删除好友关系但保留历史会话
export function deleteFriend(token: string, friendUserID: string): Promise<void> {
    return request(
        `/friends/${friendUserID}`,
        {method: 'DELETE'},
        token,
    )
}

// listBlockedUsers 分页查询当前用户拉黑的账号
export async function listBlockedUsers(
    token: string,
    beforeID = '',
): Promise<BlockedUserPage> {
    const params = pageParams(20, beforeID)
    return request<BlockedUserPage>(`/user-blocks?${params}`, {}, token)
}

// blockUser 拉黑指定用户并阻止新的私聊消息
export function blockUser(token: string, targetUserID: string): Promise<void> {
    return request(
        `/users/${targetUserID}/block`,
        {method: 'POST'},
        token,
    )
}

// unblockUser 解除对指定用户的拉黑
export function unblockUser(token: string, targetUserID: string): Promise<void> {
    return request(
        `/users/${targetUserID}/block`,
        {method: 'DELETE'},
        token,
    )
}
