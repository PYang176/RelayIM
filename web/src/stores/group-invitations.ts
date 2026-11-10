import {defineStore} from 'pinia'
import {ref, shallowRef} from 'vue'

import * as invitationApi from '@/api/group-invitations'
import {useAuthStore} from '@/stores/auth'
import type {
    GroupPendingInvitation,
    InvitationAcceptance,
    ReceivedGroupInvitation,
} from '@/types/api'

export const useGroupInvitationStore = defineStore('group-invitations', () => {
    const auth = useAuthStore()
    const invitations = ref<ReceivedGroupInvitation[]>([])
    const groupHistory = ref<Record<string, GroupPendingInvitation[]>>({})
    const nextCursor = shallowRef('')
    const hasMore = shallowRef(false)
    const groupHasMore = ref<Record<string, boolean>>({})
    const groupNextCursor = ref<Record<string, string>>({})
    const loading = shallowRef(false)
    const loadingMore = shallowRef(false)
    const actionBusy = shallowRef(false)
    const error = shallowRef('')

    // reportError 记录群邀请功能的统一错误信息
    function reportError(cause: unknown, fallback: string) {
        error.value = cause instanceof Error ? cause.message : fallback
    }

    // loadReceived 加载当前用户收到的群邀请
    async function loadReceived() {
        loading.value = true
        clearError()
        try {
            const page = await invitationApi.listReceivedInvitations(auth.token)
            invitations.value = page.invitations
            nextCursor.value = page.next_cursor
            hasMore.value = page.has_more
        } catch (cause) {
            reportError(cause, '加载群邀请失败')
        } finally {
            loading.value = false
        }
    }

    // loadMoreReceived 加载群邀请列表下一页
    async function loadMoreReceived() {
        if (!hasMore.value || !nextCursor.value || loadingMore.value) return
        loadingMore.value = true
        try {
            const page = await invitationApi.listReceivedInvitations(
                auth.token,
                nextCursor.value,
            )
            invitations.value = [...invitations.value, ...page.invitations]
            nextCursor.value = page.next_cursor
            hasMore.value = page.has_more
        } catch (cause) {
            reportError(cause, '加载更多群邀请失败')
        } finally {
            loadingMore.value = false
        }
    }

    // loadGroupInvitations 加载指定群的邀请记录
    async function loadGroupInvitations(
        conversationID: string,
        reset = true,
    ) {
        if (reset) {
            groupHistory.value = {
                ...groupHistory.value,
                [conversationID]: [],
            }
        }
        try {
            const page = await invitationApi.listGroupInvitations(
                auth.token,
                conversationID,
                reset ? '' : (groupNextCursor.value[conversationID] ?? ''),
            )
            groupHistory.value = {
                ...groupHistory.value,
                [conversationID]: reset
                    ? page.invitations
                    : [
                        ...(groupHistory.value[conversationID] ?? []),
                        ...page.invitations,
                    ],
            }
            groupNextCursor.value = {
                ...groupNextCursor.value,
                [conversationID]: page.next_cursor,
            }
            groupHasMore.value = {
                ...groupHasMore.value,
                [conversationID]: page.has_more,
            }
        } catch (cause) {
            reportError(cause, '加载群待处理邀请失败')
        }
    }

    // acceptInvitation 接受群邀请并刷新会话数据
    async function acceptInvitation(
        invitationID: string,
        responseRemark = '',
    ): Promise<InvitationAcceptance | null> {
        actionBusy.value = true
        clearError()
        try {
            const result = await invitationApi.acceptInvitation(
                auth.token,
                invitationID,
                responseRemark,
            )
            invitations.value = invitations.value.filter(
                (item) => item.invitation_id !== invitationID,
            )
            return result
        } catch (cause) {
            reportError(cause, '接受群邀请失败')
            return null
        } finally {
            actionBusy.value = false
        }
    }

    // rejectInvitation 拒绝群邀请并刷新列表
    async function rejectInvitation(
        invitationID: string,
        responseRemark = '',
    ) {
        actionBusy.value = true
        clearError()
        try {
            await invitationApi.rejectInvitation(
                auth.token,
                invitationID,
                responseRemark,
            )
            invitations.value = invitations.value.filter(
                (item) => item.invitation_id !== invitationID,
            )
        } catch (cause) {
            reportError(cause, '拒绝群邀请失败')
        } finally {
            actionBusy.value = false
        }
    }

    // cancelInvitation 取消群内待处理邀请
    async function cancelInvitation(
        conversationID: string,
        invitationID: string,
    ) {
        actionBusy.value = true
        clearError()
        try {
            await invitationApi.cancelInvitation(
                auth.token,
                conversationID,
                invitationID,
            )
            await loadGroupInvitations(conversationID, true)
        } catch (cause) {
            reportError(cause, '取消群邀请失败')
        } finally {
            actionBusy.value = false
        }
    }

    // clearError 清空群邀请功能错误信息
    function clearError() {
        error.value = ''
    }

    // reset 重置群邀请 Store 状态
    function reset() {
        invitations.value = []
        groupHistory.value = {}
        nextCursor.value = ''
        hasMore.value = false
        groupNextCursor.value = {}
        groupHasMore.value = {}
        error.value = ''
    }

    return {
        invitations,
        groupHistory,
        hasMore,
        groupHasMore,
        loading,
        loadingMore,
        actionBusy,
        error,
        loadReceived,
        loadMoreReceived,
        loadGroupInvitations,
        acceptInvitation,
        rejectInvitation,
        cancelInvitation,
        clearError,
        reset,
    }
})
