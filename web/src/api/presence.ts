import {request} from './http'

interface PresenceStatus {
    user_id: string
    online: boolean
}

// queryPresence 批量查询用户在线状态
export async function queryPresence(
    token: string,
    userIds: string[],
): Promise<Record<string, boolean>> {
    if (userIds.length === 0) return {}
    const response = await request<{presence: PresenceStatus[]}>(
        '/presence/query',
        {
            method: 'POST',
            body: JSON.stringify({user_ids: userIds}),
        },
        token,
    )
    return Object.fromEntries(
        response.presence.map((item) => [item.user_id, item.online]),
    )
}
