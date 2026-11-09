import {request} from './http'
import type {
    GroupInvitationPage,
    GroupPendingInvitationPage,
    InvitationAcceptance,
} from '@/types/api'

function pageParams(beforeID = ''): URLSearchParams {
    const params = new URLSearchParams({limit: '20'})
    if (beforeID) params.set('before_id', beforeID)
    return params
}

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

export function acceptInvitation(
    token: string,
    invitationID: string,
): Promise<InvitationAcceptance> {
    return request(
        `/group-invitations/${invitationID}/accept`,
        {method: 'POST'},
        token,
    )
}

export function rejectInvitation(
    token: string,
    invitationID: string,
): Promise<void> {
    return request(
        `/group-invitations/${invitationID}/reject`,
        {method: 'POST'},
        token,
    )
}

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
