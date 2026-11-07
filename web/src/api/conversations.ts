import {request} from './http'
import type {Conversation} from '@/types/api'

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
