import {request} from './http'
import type {
    BlockedUserPage,
    FriendPage,
    FriendRequestPage,
    FriendUser,
} from '@/types/api'

function pageParams(limit = 20, beforeID = ''): URLSearchParams {
    const params = new URLSearchParams({limit: String(limit)})
    if (beforeID) params.set('before_id', beforeID)
    return params
}

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

export function acceptFriendRequest(token: string, requestID: string): Promise<void> {
    return request(
        `/friend-requests/${requestID}/accept`,
        {method: 'POST'},
        token,
    )
}

export function rejectFriendRequest(token: string, requestID: string): Promise<void> {
    return request(
        `/friend-requests/${requestID}/reject`,
        {method: 'POST'},
        token,
    )
}

export async function listFriends(
    token: string,
    beforeID = '',
): Promise<FriendPage> {
    const params = pageParams(20, beforeID)
    return request<FriendPage>(`/friends?${params}`, {}, token)
}

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

export function deleteFriend(token: string, friendUserID: string): Promise<void> {
    return request(
        `/friends/${friendUserID}`,
        {method: 'DELETE'},
        token,
    )
}

export async function listBlockedUsers(
    token: string,
    beforeID = '',
): Promise<BlockedUserPage> {
    const params = pageParams(20, beforeID)
    return request<BlockedUserPage>(`/user-blocks?${params}`, {}, token)
}

export function blockUser(token: string, targetUserID: string): Promise<void> {
    return request(
        `/users/${targetUserID}/block`,
        {method: 'POST'},
        token,
    )
}

export function unblockUser(token: string, targetUserID: string): Promise<void> {
    return request(
        `/users/${targetUserID}/block`,
        {method: 'DELETE'},
        token,
    )
}
