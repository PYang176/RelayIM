import {ref, shallowRef} from 'vue'

import * as messageApi from '@/api/messages'
import {useAuthStore} from '@/stores/auth'
import type {Message} from '@/types/api'

// useConversationFiles 管理会话图片和文件列表的分页状态
export function useConversationFiles() {
    const auth = useAuthStore()
    const files = ref<Message[]>([])
    const loading = shallowRef(false)
    const loadingMore = shallowRef(false)
    const hasMore = shallowRef(false)
    const beforeSeq = shallowRef(0)
    const error = shallowRef('')
    let conversationId = ''
    let generation = 0

    // load 加载会话最新一页图片和文件
    async function load(targetConversationId: string) {
        const currentGeneration = ++generation
        conversationId = targetConversationId
        files.value = []
        beforeSeq.value = 0
        hasMore.value = false
        error.value = ''
        loading.value = true
        try {
            const page = await messageApi.listFiles(
                auth.token,
                targetConversationId,
                0,
                30,
            )
            if (
                currentGeneration !== generation ||
                conversationId !== targetConversationId
            ) {
                return
            }
            files.value = page.files
            beforeSeq.value = page.next_before_seq
            hasMore.value = page.has_more
        } catch (cause) {
            if (currentGeneration === generation) {
                error.value =
                    cause instanceof Error ? cause.message : '加载会话文件失败'
            }
        } finally {
            if (currentGeneration === generation) {
                loading.value = false
            }
        }
    }

    // loadMore 加载更早的会话图片和文件
    async function loadMore() {
        if (loadingMore.value || !hasMore.value || !conversationId) return
        const currentGeneration = generation
        const targetConversationId = conversationId
        loadingMore.value = true
        error.value = ''
        try {
            const page = await messageApi.listFiles(
                auth.token,
                targetConversationId,
                beforeSeq.value,
                30,
            )
            if (
                currentGeneration !== generation ||
                conversationId !== targetConversationId
            ) {
                return
            }
            files.value = [...files.value, ...page.files]
            beforeSeq.value = page.next_before_seq
            hasMore.value = page.has_more
        } catch (cause) {
            if (currentGeneration === generation) {
                error.value =
                    cause instanceof Error ? cause.message : '加载更多文件失败'
            }
        } finally {
            if (currentGeneration === generation) {
                loadingMore.value = false
            }
        }
    }

    // reset 清空附件列表和分页状态
    function reset() {
        generation += 1
        conversationId = ''
        files.value = []
        loading.value = false
        loadingMore.value = false
        hasMore.value = false
        beforeSeq.value = 0
        error.value = ''
    }

    return {
        files,
        loading,
        loadingMore,
        hasMore,
        error,
        load,
        loadMore,
        reset,
    }
}
