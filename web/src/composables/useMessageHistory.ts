import {computed, ref, shallowRef} from 'vue'

import * as messageApi from '@/api/messages'
import {useAuthStore} from '@/stores/auth'
import type {
    Message,
    MessageSearchResult,
} from '@/types/api'

// useMessageHistory 创建聊天记录搜索和分页状态
export function useMessageHistory() {
    const auth = useAuthStore()
    const recentMessages = ref<Message[]>([])
    const searchResults = ref<MessageSearchResult[]>([])
    const query = shallowRef('')
    const activeQuery = shallowRef('')
    const loading = shallowRef(false)
    const searching = shallowRef(false)
    const loadingMore = shallowRef(false)
    const historyHasMore = shallowRef(false)
    const searchHasMore = shallowRef(false)
    const historyBeforeSeq = shallowRef(0)
    const searchBeforeSeq = shallowRef(0)
    const error = shallowRef('')
    let conversationId = ''
    let generation = 0

    const hasMore = computed(() =>
        activeQuery.value ? searchHasMore.value : historyHasMore.value,
    )
    const showingSearch = computed(() => Boolean(activeQuery.value))

    // loadRecent 加载会话最近消息作为历史窗口基线
    async function loadRecent(targetConversationId: string) {
        const currentGeneration = ++generation
        conversationId = targetConversationId
        query.value = ''
        activeQuery.value = ''
        recentMessages.value = []
        searchResults.value = []
        historyBeforeSeq.value = 0
        searchBeforeSeq.value = 0
        historyHasMore.value = false
        searchHasMore.value = false
        error.value = ''
        loading.value = true
        try {
            const page = await messageApi.listMessages(
                auth.token,
                targetConversationId,
                {limit: 30},
            )
            if (
                currentGeneration !== generation ||
                conversationId !== targetConversationId
            ) {
                return
            }
            recentMessages.value = [...page.messages].reverse()
            historyHasMore.value = page.has_more
            historyBeforeSeq.value = page.messages[0]?.seq ?? 0
        } catch (cause) {
            if (currentGeneration === generation) {
                error.value =
                    cause instanceof Error ? cause.message : '加载聊天记录失败'
            }
        } finally {
            if (currentGeneration === generation) {
                loading.value = false
            }
        }
    }

    // search 执行会话内消息搜索并重置分页
    async function search(rawQuery: string) {
        const value = rawQuery.trim()
        query.value = value
        error.value = ''
        if (!value) {
            clearSearch()
            return
        }
        if ([...value].length < 2) {
            activeQuery.value = ''
            searchResults.value = []
            searchHasMore.value = false
            error.value = '搜索词至少需要 2 个字符'
            return
        }

        const currentGeneration = ++generation
        const targetConversationId = conversationId
        searching.value = true
        activeQuery.value = value
        searchResults.value = []
        searchBeforeSeq.value = 0
        searchHasMore.value = false
        try {
            const page = await messageApi.searchMessages(
                auth.token,
                targetConversationId,
                value,
            )
            if (
                currentGeneration !== generation ||
                conversationId !== targetConversationId
            ) {
                return
            }
            activeQuery.value = value
            searchResults.value = page.messages
            searchBeforeSeq.value = page.next_before_seq
            searchHasMore.value = page.has_more
        } catch (cause) {
            if (currentGeneration === generation) {
                activeQuery.value = value
                searchResults.value = []
                searchHasMore.value = false
                error.value =
                    cause instanceof Error ? cause.message : '搜索聊天记录失败'
            }
        } finally {
            if (currentGeneration === generation) {
                searching.value = false
            }
        }
    }

    // loadMore 加载当前聊天记录页的下一页
    async function loadMore() {
        if (loadingMore.value || !hasMore.value) return
        const currentGeneration = generation
        const targetConversationId = conversationId
        loadingMore.value = true
        error.value = ''
        try {
            if (activeQuery.value) {
                const page = await messageApi.searchMessages(
                    auth.token,
                    targetConversationId,
                    activeQuery.value,
                    searchBeforeSeq.value,
                )
                if (
                    currentGeneration !== generation ||
                    conversationId !== targetConversationId
                ) {
                    return
                }
                searchResults.value = [
                    ...searchResults.value,
                    ...page.messages,
                ]
                searchBeforeSeq.value = page.next_before_seq
                searchHasMore.value = page.has_more
                return
            }

            const page = await messageApi.listMessages(
                auth.token,
                targetConversationId,
                {
                    beforeSeq: historyBeforeSeq.value || undefined,
                    limit: 30,
                },
            )
            if (
                currentGeneration !== generation ||
                conversationId !== targetConversationId
            ) {
                return
            }
            recentMessages.value = [
                ...recentMessages.value,
                ...[...page.messages].reverse(),
            ]
            historyBeforeSeq.value = page.messages[0]?.seq ?? 0
            historyHasMore.value = page.has_more
        } catch (cause) {
            if (currentGeneration === generation) {
                error.value =
                    cause instanceof Error ? cause.message : '加载更多记录失败'
            }
        } finally {
            if (currentGeneration === generation) {
                loadingMore.value = false
            }
        }
    }

    // clearSearch 清空搜索并回到最近消息视图
    function clearSearch() {
        generation += 1
        query.value = ''
        activeQuery.value = ''
        searchResults.value = []
        searchBeforeSeq.value = 0
        searchHasMore.value = false
        searching.value = false
        error.value = ''
    }

    // reset 重置聊天记录窗口与搜索状态
    function reset() {
        generation += 1
        conversationId = ''
        recentMessages.value = []
        searchResults.value = []
        query.value = ''
        activeQuery.value = ''
        loading.value = false
        searching.value = false
        loadingMore.value = false
        historyHasMore.value = false
        searchHasMore.value = false
        historyBeforeSeq.value = 0
        searchBeforeSeq.value = 0
        error.value = ''
    }

    return {
        recentMessages,
        searchResults,
        query,
        activeQuery,
        loading,
        searching,
        loadingMore,
        hasMore,
        showingSearch,
        error,
        loadRecent,
        search,
        loadMore,
        clearSearch,
        reset,
    }
}
