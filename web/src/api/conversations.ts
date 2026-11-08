import {request} from './http'
import type {Conversation, ConversationMember} from '@/types/api'

export async function listConversations(token: string): Promise<Conversation[]> {
    const response = await request<{ conversations: Conversation[] }>(
        '/conversations',
        {},
        token,
    )
    return response.conversations
}

export function createDirect(
    token: string,
    peerUserId: string,
): Promise<Conversation> {
    return request(
        '/conversations/private',
        {
            method: 'POST',
            body: JSON.stringify({peer_user_id: peerUserId}),
        },
        token,
    )
}

export function createGroup(
    token: string,
    name: string,
    memberUserIds: string[],
): Promise<Conversation> {
    return request(
        '/conversations/groups',
        {
            method: 'POST',
            body: JSON.stringify({
                name,
                member_user_ids: memberUserIds,
            }),
        },
        token,
    )
}

export async function listMembers(
    token: string,
    conversationId: string,
): Promise<ConversationMember[]> {
    const response = await request<{members: ConversationMember[]}>(
        `/conversations/${conversationId}/members`,
        {},
        token,
    )
    return response.members
}

export async function inviteGroupMembers(
    token: string,
    conversationId: string,
    memberUserIds: string[],
): Promise<number> {
    const response = await request<{member_count: number}>(
        `/conversations/${conversationId}/members`,
        {
            method: 'POST',
            body: JSON.stringify({member_user_ids: memberUserIds}),
        },
        token,
    )
    return response.member_count
}

export function removeGroupMember(
    token: string,
    conversationId: string,
    userId: string,
): Promise<void> {
    return request(
        `/conversations/${conversationId}/members/${userId}`,
        {method: 'DELETE'},
        token,
    )
}

export function setGroupMemberRole(
    token: string,
    conversationId: string,
    userId: string,
    role: 1 | 2,
): Promise<void> {
    return request(
        `/conversations/${conversationId}/members/${userId}/role`,
        {
            method: 'PUT',
            body: JSON.stringify({role}),
        },
        token,
    )
}

export function transferGroupOwner(
    token: string,
    conversationId: string,
    userId: string,
): Promise<void> {
    return request(
        `/conversations/${conversationId}/transfer-owner`,
        {
            method: 'POST',
            body: JSON.stringify({user_id: userId}),
        },
        token,
    )
}

export function dissolveGroup(
    token: string,
    conversationId: string,
): Promise<void> {
    return request(
        `/conversations/${conversationId}/dissolve`,
        {method: 'POST'},
        token,
    )
}

export function updateGroupProfile(
    token: string,
    conversationId: string,
    name: string,
    avatarUrl: string,
): Promise<{id: string; name: string; avatar_url: string}> {
    return request(
        `/conversations/${conversationId}/profile`,
        {
            method: 'PUT',
            body: JSON.stringify({
                name,
                avatar_url: avatarUrl,
            }),
        },
        token,
    )
}
