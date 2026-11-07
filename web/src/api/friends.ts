import {request} from './http'
import type {Friend, FriendRequest, FriendUser} from '@/types/api'

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
): Promise<FriendRequest[]> {
    const response = await request<{ requests: FriendRequest[] }>(
        '/friend-requests/received',
        {},
        token,
    )
    return response.requests
}

export async function listSentRequests(
    token: string,
): Promise<FriendRequest[]> {
    const response = await request<{ requests: FriendRequest[] }>(
        '/friend-requests/sent',
        {},
        token,
    )
    return response.requests
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

export async function listFriends(token: string): Promise<Friend[]> {
    const response = await request<{ friends: Friend[] }>('/friends', {}, token)
    return response.friends
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
