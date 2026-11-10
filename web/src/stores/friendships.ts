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
    const friendNextCursor = shallowRef('')
    const friendHasMore = shallowRef(false)
    const receivedNextCursor = shallowRef('')
    const receivedHasMore = shallowRef(false)
    const sentNextCursor = shallowRef('')
    const sentHasMore = shallowRef(false)
    const blockedUsers = ref<BlockedUser[]>([])
    const blockedNextCursor = shallowRef('')
    const blockedHasMore = shallowRef(false)
    const searchResults = ref<FriendUser[]>([])
    const loading = shallowRef(false)
    const searching = shallowRef(false)
    const actionBusy = shallowRef(false)
    const loadingMore = shallowRef(false)
    const error = shallowRef('')

    // reportError 记录好友功能的统一错误信息
    function reportError(cause: unknown, fallback: string) {
        error.value = cause instanceof Error ? cause.message : fallback
    }

    // clearError 清空好友功能错误信息
    function clearError() {
        error.value = ''
    }

    // loadAll 并行加载好友、申请和拉黑列表
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
            friends.value = friendList.friends
            friendNextCursor.value = friendList.next_cursor
            friendHasMore.value = friendList.has_more
            receivedRequests.value = received.requests
            receivedNextCursor.value = received.next_cursor
            receivedHasMore.value = received.has_more
            sentRequests.value = sent.requests
            sentNextCursor.value = sent.next_cursor
            sentHasMore.value = sent.has_more
            blockedUsers.value = blocked.blocked_users
            blockedNextCursor.value = blocked.next_cursor
            blockedHasMore.value = blocked.has_more
        } catch (cause) {
            reportError(cause, '加载好友信息失败')
        } finally {
            loading.value = false
        }
    }

    // search 搜索可添加的用户
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

    // loadMoreFriends 加载好友列表下一页
    async function loadMoreFriends() {
        if (!friendHasMore.value || !friendNextCursor.value || loadingMore.value) {
            return
        }
        loadingMore.value = true
        try {
            const page = await friendApi.listFriends(
                auth.token,
                friendNextCursor.value,
            )
            friends.value = [...friends.value, ...page.friends]
            friendNextCursor.value = page.next_cursor
            friendHasMore.value = page.has_more
        } catch (cause) {
            reportError(cause, '加载更多好友失败')
        } finally {
            loadingMore.value = false
        }
    }

    // loadMoreReceivedRequests 加载收到的申请下一页
    async function loadMoreReceivedRequests() {
        if (
            !receivedHasMore.value ||
            !receivedNextCursor.value ||
            loadingMore.value
        ) {
            return
        }
        loadingMore.value = true
        try {
            const page = await friendApi.listReceivedRequests(
                auth.token,
                receivedNextCursor.value,
            )
            receivedRequests.value = [...receivedRequests.value, ...page.requests]
            receivedNextCursor.value = page.next_cursor
            receivedHasMore.value = page.has_more
        } catch (cause) {
            reportError(cause, '加载更多收到申请失败')
        } finally {
            loadingMore.value = false
        }
    }

    // loadMoreSentRequests 加载发出的申请下一页
    async function loadMoreSentRequests() {
        if (!sentHasMore.value || !sentNextCursor.value || loadingMore.value) {
            return
        }
        loadingMore.value = true
        try {
            const page = await friendApi.listSentRequests(
                auth.token,
                sentNextCursor.value,
            )
            sentRequests.value = [...sentRequests.value, ...page.requests]
            sentNextCursor.value = page.next_cursor
            sentHasMore.value = page.has_more
        } catch (cause) {
            reportError(cause, '加载更多发出申请失败')
        } finally {
            loadingMore.value = false
        }
    }

    // loadMoreBlockedUsers 加载拉黑列表下一页
    async function loadMoreBlockedUsers() {
        if (!blockedHasMore.value || !blockedNextCursor.value || loadingMore.value) {
            return
        }
        loadingMore.value = true
        try {
            const page = await friendApi.listBlockedUsers(
                auth.token,
                blockedNextCursor.value,
            )
            blockedUsers.value = [...blockedUsers.value, ...page.blocked_users]
            blockedNextCursor.value = page.next_cursor
            blockedHasMore.value = page.has_more
        } catch (cause) {
            reportError(cause, '加载更多拉黑用户失败')
        } finally {
            loadingMore.value = false
        }
    }

    // sendRequest 发送好友申请并刷新申请列表
    async function sendRequest(
        targetUserID: string,
        remark: string,
    ): Promise<boolean> {
        actionBusy.value = true
        clearError()
        try {
            await friendApi.sendFriendRequest(auth.token, targetUserID, remark)
            searchResults.value = searchResults.value.filter(
                (user) => user.id !== targetUserID,
            )
            await loadAll()
            return true
        } catch (cause) {
            reportError(cause, '发送好友申请失败')
            return false
        } finally {
            actionBusy.value = false
        }
    }

    // acceptRequest 接受好友申请并刷新好友数据
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

    // rejectRequest 拒绝好友申请并刷新列表
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

    // updateRemark 更新好友备注并同步缓存
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

    // removeFriend 删除好友并刷新好友列表
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

    // blockUser 拉黑用户并刷新关系和列表
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

    // unblockUser 解除拉黑并刷新关系和列表
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

    // reset 重置好友 Store 状态
    function reset() {
        friends.value = []
        receivedRequests.value = []
        sentRequests.value = []
        friendNextCursor.value = ''
        friendHasMore.value = false
        receivedNextCursor.value = ''
        receivedHasMore.value = false
        sentNextCursor.value = ''
        sentHasMore.value = false
        blockedUsers.value = []
        blockedNextCursor.value = ''
        blockedHasMore.value = false
        searchResults.value = []
        error.value = ''
    }

    return {
        friends,
        receivedRequests,
        sentRequests,
        friendHasMore,
        receivedHasMore,
        sentHasMore,
        blockedHasMore,
        blockedUsers,
        searchResults,
        loading,
        searching,
        actionBusy,
        loadingMore,
        error,
        loadAll,
        search,
        loadMoreFriends,
        loadMoreReceivedRequests,
        loadMoreSentRequests,
        loadMoreBlockedUsers,
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
