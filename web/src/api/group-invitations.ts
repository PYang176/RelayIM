import {request} from './http'
import type {
    GroupInvitationPage,
    GroupPendingInvitationPage,
    InvitationAcceptance,
} from '@/types/api'

// pageParams 构造群邀请分页查询参数
function pageParams(beforeID = ''): URLSearchParams {
    const params = new URLSearchParams({limit: '20'})
    if (beforeID) params.set('before_id', beforeID)
    return params
}

// listReceivedInvitations 分页查询当前用户收到的群邀请
export function listReceivedInvitations(
    token: string,
    beforeID = '',
): Promise<GroupInvitationPage> {
    const params = pageParams(beforeID)
    return request<GroupInvitationPage>(
        `/group-invitations?${params}`,
        {},
        token,
    )
}

// acceptInvitation 接受群邀请并提交审批回复
export function acceptInvitation(
    token: string,
    invitationID: string,
    responseRemark: string,
): Promise<InvitationAcceptance> {
    return request(
        `/group-invitations/${invitationID}/accept`,
        {
            method: 'POST',
            body: JSON.stringify({response_remark: responseRemark}),
        },
        token,
    )
}

// rejectInvitation 拒绝群邀请并提交审批回复
export function rejectInvitation(
    token: string,
    invitationID: string,
    responseRemark: string,
): Promise<void> {
    return request(
        `/group-invitations/${invitationID}/reject`,
        {
            method: 'POST',
            body: JSON.stringify({response_remark: responseRemark}),
        },
        token,
    )
}

// cancelInvitation 取消群内仍待处理的邀请
export function cancelInvitation(
    token: string,
    conversationID: string,
    invitationID: string,
): Promise<void> {
    return request(
        `/conversations/${conversationID}/invitations/${invitationID}`,
        {method: 'DELETE'},
        token,
    )
}

// listGroupInvitations 分页查询指定群的邀请记录
export function listGroupInvitations(
    token: string,
    conversationID: string,
    beforeID = '',
): Promise<GroupPendingInvitationPage> {
    const params = pageParams(beforeID)
    return request<GroupPendingInvitationPage>(
        `/conversations/${conversationID}/invitations?${params}`,
        {},
        token,
    )
}
