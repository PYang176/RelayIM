import {request} from './http'
import type {
    Conversation,
    ConversationMember,
    ConversationPage,
} from '@/types/api'

// listConversations 分页查询当前用户的会话列表
export function listConversations(
    token: string,
    options: {cursor?: string; query?: string} = {},
): Promise<ConversationPage> {
    const params = new URLSearchParams({limit: '20'})
    if (options.cursor) params.set('cursor', options.cursor)
    if (options.query) params.set('q', options.query)
    return request<ConversationPage>(`/conversations?${params}`, {}, token)
}

// getConversation 读取单个会话摘要和成员信息
export function getConversation(
    token: string,
    conversationId: string,
): Promise<Conversation> {
    return request(`/conversations/${conversationId}`, {}, token)
}

// setConversationPinned 设置当前用户对会话的个人置顶状态
export function setConversationPinned(
    token: string,
    conversationId: string,
    pinned: boolean,
): Promise<void> {
    return request(
        `/conversations/${conversationId}/pin`,
        {
            method: 'PUT',
            body: JSON.stringify({pinned}),
        },
        token,
    )
}

// createDirect 创建或复用两个用户之间的私聊会话
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

// createGroup 创建群聊并返回新会话
export function createGroup(
    token: string,
    name: string,
    memberUserIds: string[],
    inviteRemark: string,
): Promise<Conversation> {
    return request(
        '/conversations/groups',
        {
            method: 'POST',
            body: JSON.stringify({
                name,
                member_user_ids: memberUserIds,
                invite_remark: inviteRemark,
            }),
        },
        token,
    )
}

// listMembers 查询群成员及其角色信息
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

// inviteGroupMembers 发送群邀请并附带邀请备注
export async function inviteGroupMembers(
    token: string,
    conversationId: string,
    memberUserIds: string[],
    inviteRemark: string,
): Promise<number> {
    const response = await request<{invited_count: number}>(
        `/conversations/${conversationId}/members`,
        {
            method: 'POST',
            body: JSON.stringify({
                member_user_ids: memberUserIds,
                invite_remark: inviteRemark,
            }),
        },
        token,
    )
    return response.invited_count
}

// removeGroupMember 将指定成员移出群聊
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

// setGroupMemberRole 调整群成员为管理员或普通成员
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

// transferGroupOwner 把群主身份转让给指定成员
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

// dissolveGroup 解散群聊并关闭会话写入
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

// updateGroupProfile 更新群名称和群头像
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
