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
