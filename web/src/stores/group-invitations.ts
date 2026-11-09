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
    const groupPending = ref<Record<string, GroupPendingInvitation[]>>({})
    const nextCursor = shallowRef('')
    const hasMore = shallowRef(false)
    const groupHasMore = ref<Record<string, boolean>>({})
    const groupNextCursor = ref<Record<string, string>>({})
    const loading = shallowRef(false)
    const loadingMore = shallowRef(false)
    const actionBusy = shallowRef(false)
    const error = shallowRef('')

    function reportError(cause: unknown, fallback: string) {
        error.value = cause instanceof Error ? cause.message : fallback
    }

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

    async function loadGroupInvitations(
        conversationID: string,
        reset = true,
    ) {
        if (reset) {
            groupPending.value = {
                ...groupPending.value,
                [conversationID]: [],
            }
        }
        try {
            const page = await invitationApi.listGroupInvitations(
                auth.token,
                conversationID,
                reset ? '' : (groupNextCursor.value[conversationID] ?? ''),
            )
            groupPending.value = {
                ...groupPending.value,
                [conversationID]: reset
                    ? page.invitations
                    : [
                        ...(groupPending.value[conversationID] ?? []),
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

    async function acceptInvitation(
        invitationID: string,
    ): Promise<InvitationAcceptance | null> {
        actionBusy.value = true
        clearError()
        try {
            const result = await invitationApi.acceptInvitation(
                auth.token,
                invitationID,
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

    async function rejectInvitation(invitationID: string) {
        actionBusy.value = true
        clearError()
        try {
            await invitationApi.rejectInvitation(auth.token, invitationID)
            invitations.value = invitations.value.filter(
                (item) => item.invitation_id !== invitationID,
            )
        } catch (cause) {
            reportError(cause, '拒绝群邀请失败')
        } finally {
            actionBusy.value = false
        }
    }

    function clearError() {
        error.value = ''
    }

    function reset() {
        invitations.value = []
        groupPending.value = {}
        nextCursor.value = ''
        hasMore.value = false
        groupNextCursor.value = {}
        groupHasMore.value = {}
        error.value = ''
    }

    return {
        invitations,
        groupPending,
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
        clearError,
        reset,
    }
})
