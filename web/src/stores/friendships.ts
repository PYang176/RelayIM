import {defineStore} from 'pinia'
import {ref, shallowRef} from 'vue'

import * as friendApi from '@/api/friends'
import {useAuthStore} from '@/stores/auth'
import type {BlockedUser, Friend, FriendRequest, FriendUser} from '@/types/api'

export const useFriendshipStore = defineStore('friendships', () => {
    const auth = useAuthStore()
    const friends = ref<Friend[]>([])
    const receivedRequests = ref<FriendRequest[]>([])
    const sentRequests = ref<FriendRequest[]>([])
    const blockedUsers = ref<BlockedUser[]>([])
    const searchResults = ref<FriendUser[]>([])
    const loading = shallowRef(false)
    const searching = shallowRef(false)
    const actionBusy = shallowRef(false)
    const error = shallowRef('')

    function reportError(cause: unknown, fallback: string) {
        error.value = cause instanceof Error ? cause.message : fallback
    }

    function clearError() {
        error.value = ''
    }

    async function loadAll() {
        loading.value = true
        clearError()
        try {
            const [friendList, received, sent, blocked] = await Promise.all([
                friendApi.listFriends(auth.token),
                friendApi.listReceivedRequests(auth.token),
                friendApi.listSentRequests(auth.token),
                friendApi.listBlockedUsers(auth.token),
            ])
            friends.value = friendList
            receivedRequests.value = received
            sentRequests.value = sent
            blockedUsers.value = blocked
        } catch (cause) {
            reportError(cause, '加载好友信息失败')
        } finally {
            loading.value = false
        }
    }

    async function search(query: string) {
        const value = query.trim()
        if (!value) {
            searchResults.value = []
            return
        }
        searching.value = true
        clearError()
        try {
            searchResults.value = await friendApi.searchUsers(auth.token, value)
        } catch (cause) {
            reportError(cause, '搜索用户失败')
        } finally {
            searching.value = false
        }
    }

    async function sendRequest(targetUserID: string, remark: string) {
        actionBusy.value = true
        clearError()
        try {
            await friendApi.sendFriendRequest(auth.token, targetUserID, remark)
            searchResults.value = searchResults.value.filter(
                (user) => user.id !== targetUserID,
            )
            await loadAll()
        } catch (cause) {
            reportError(cause, '发送好友申请失败')
        } finally {
            actionBusy.value = false
        }
    }

    async function acceptRequest(requestID: string) {
        actionBusy.value = true
        try {
            await friendApi.acceptFriendRequest(auth.token, requestID)
            await loadAll()
        } catch (cause) {
            reportError(cause, '同意好友申请失败')
        } finally {
            actionBusy.value = false
        }
    }

    async function rejectRequest(requestID: string) {
        actionBusy.value = true
        try {
            await friendApi.rejectFriendRequest(auth.token, requestID)
            await loadAll()
        } catch (cause) {
            reportError(cause, '拒绝好友申请失败')
        } finally {
            actionBusy.value = false
        }
    }

    async function updateRemark(friendUserID: string, remark: string) {
        actionBusy.value = true
        try {
            await friendApi.setFriendRemark(auth.token, friendUserID, remark)
            await loadAll()
        } catch (cause) {
            reportError(cause, '更新好友备注失败')
        } finally {
            actionBusy.value = false
        }
    }

    async function removeFriend(friendUserID: string) {
        actionBusy.value = true
        try {
            await friendApi.deleteFriend(auth.token, friendUserID)
            await loadAll()
        } catch (cause) {
            reportError(cause, '删除好友失败')
        } finally {
            actionBusy.value = false
        }
    }

    async function blockUser(targetUserID: string) {
        actionBusy.value = true
        clearError()
        try {
            await friendApi.blockUser(auth.token, targetUserID)
            await loadAll()
        } catch (cause) {
            reportError(cause, '拉黑用户失败')
        } finally {
            actionBusy.value = false
        }
    }

    async function unblockUser(targetUserID: string) {
        actionBusy.value = true
        clearError()
        try {
            await friendApi.unblockUser(auth.token, targetUserID)
            await loadAll()
        } catch (cause) {
            reportError(cause, '取消拉黑失败')
        } finally {
            actionBusy.value = false
        }
    }

    function reset() {
        friends.value = []
        receivedRequests.value = []
        sentRequests.value = []
        blockedUsers.value = []
        searchResults.value = []
        error.value = ''
    }

    return {
        friends,
        receivedRequests,
        sentRequests,
        blockedUsers,
        searchResults,
        loading,
        searching,
        actionBusy,
        error,
        loadAll,
        search,
        sendRequest,
        acceptRequest,
        rejectRequest,
        updateRemark,
        removeFriend,
        blockUser,
        unblockUser,
        clearError,
        reset,
    }
})
