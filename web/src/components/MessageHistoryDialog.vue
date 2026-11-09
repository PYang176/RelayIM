<script setup lang="ts">
import {nextTick, shallowRef, useTemplateRef, watch} from 'vue'

import {useMessageHistory} from '@/composables/useMessageHistory'
import {useAuthStore} from '@/stores/auth'
import {useChatStore} from '@/stores/chat'
import type {Message, MessageSearchResult} from '@/types/api'

const props = defineProps<{
    open: boolean
    conversationId: string
    conversationTitle: string
}>()

const emit = defineEmits<{
    close: []
}>()

const dialog = useTemplateRef<HTMLDialogElement>('dialog')
const searchInput = useTemplateRef<HTMLInputElement>('searchInput')
const {
    recentMessages,
    searchResults,
    activeQuery,
    loading,
    searching,
    loadingMore,
    hasMore,
    showingSearch,
    error: historyError,
    loadRecent,
    search,
    loadMore,
    clearSearch: clearHistorySearch,
    reset: resetHistory,
} = useMessageHistory()
const auth = useAuthStore()
const chat = useChatStore()
const searchText = shallowRef('')

watch(
    () => props.open,
    async (open) => {
        if (open) {
            dialog.value?.showModal()
            await loadRecent(props.conversationId)
            await nextTick()
            searchInput.value?.focus()
            return
        }
        dialog.value?.close()
        resetHistory()
    },
    {immediate: true},
)

function close() {
    emit('close')
}

async function submitSearch() {
    await search(searchText.value)
}

function clearSearch() {
    searchText.value = ''
    clearHistorySearch()
    void nextTick(() => searchInput.value?.focus())
}

function formatTime(value: string) {
    return new Intl.DateTimeFormat('zh-CN', {
        month: '2-digit',
        day: '2-digit',
        hour: '2-digit',
        minute: '2-digit',
    }).format(new Date(value))
}

function recentSenderName(message: Message) {
    if (message.sender_id === auth.user?.id) return '我'
    const member = chat.activeMembers.find(
        (item) => item.user_id === message.sender_id,
    )
    if (member) return member.nickname || member.username
    if (chat.activeConversation?.conversation_type === 1) {
        return props.conversationTitle
    }
    return '会话成员'
}

function recentContent(message: Message) {
    if (message.recalled) return '该消息已撤回'
    if (message.message_type === 2) {
        return message.content ? `[图片] ${message.content}` : '[图片]'
    }
    if (message.message_type === 3) {
        const filename = message.attachment_name || '聊天文件'
        return message.content ? `[文件] ${filename} · ${message.content}` : `[文件] ${filename}`
    }
    return message.content
}

function searchSenderName(message: MessageSearchResult) {
    return message.sender_name || '会话成员'
}
</script>

<template>
    <dialog
            ref="dialog"
            class="message-history-dialog"
            aria-labelledby="message-history-title"
            @cancel.prevent="close"
            @close="close"
    >
        <section class="message-history-card">
            <header class="message-history-header">
                <div>
                    <h2 id="message-history-title">聊天记录</h2>
                    <p>{{ conversationTitle }} · 最近消息与正文搜索</p>
                </div>
                <button
                        class="icon-button"
                        aria-label="关闭聊天记录"
                        type="button"
                        @click="close"
                >
                    ×
                </button>
            </header>

            <form class="message-history-search" @submit.prevent="submitSearch">
                <label>
                    <span class="sr-only">搜索聊天记录</span>
                    <input
                            ref="searchInput"
                            v-model="searchText"
                            autocomplete="off"
                            maxlength="64"
                            placeholder="搜索聊天记录，至少输入 2 个字符"
                            type="search"
                    />
                </label>
                <button class="primary-button compact" type="submit">
                    搜索
                </button>
                <button
                        v-if="showingSearch"
                        class="ghost-button"
                        type="button"
                        @click="clearSearch"
                >
                    返回最近记录
                </button>
            </form>

            <p v-if="historyError" class="form-error" role="alert">
                {{ historyError }}
            </p>
            <p
                    v-if="loading || searching"
                    class="panel-hint"
                    role="status"
                    aria-busy="true"
            >
                {{ searching ? '搜索聊天记录中…' : '加载聊天记录中…' }}
            </p>

            <div class="message-history-results">
                <template v-if="showingSearch">
                    <article
                            v-for="message in searchResults"
                            :key="message.message_id"
                            class="message-history-entry"
                    >
                        <header>
                            <strong>{{ searchSenderName(message) }}</strong>
                            <time>{{ formatTime(message.created_at) }}</time>
                            <span>#{{ message.seq }}</span>
                        </header>
                        <p>{{ message.content }}</p>
                    </article>
                    <p
                            v-if="
                                !searching &&
                                searchResults.length === 0
                            "
                            class="panel-hint"
                            role="status"
                    >
                        没有找到包含“{{ activeQuery }}”的文字消息。
                    </p>
                </template>
                <template v-else>
                    <article
                            v-for="message in recentMessages"
                            :key="message.message_id"
                            class="message-history-entry"
                    >
                        <header>
                            <strong>{{ recentSenderName(message) }}</strong>
                            <time>{{ formatTime(message.created_at) }}</time>
                            <span>#{{ message.seq }}</span>
                        </header>
                        <p>{{ recentContent(message) }}</p>
                    </article>
                    <p
                            v-if="
                                !loading &&
                                recentMessages.length === 0
                            "
                            class="panel-hint"
                            role="status"
                    >
                        当前会话还没有可查看的历史消息。
                    </p>
                </template>
            </div>

            <footer class="message-history-footer">
                <button
                        v-if="hasMore"
                        :disabled="loadingMore"
                        class="ghost-button"
                        type="button"
                        @click="loadMore"
                >
                    {{
                        loadingMore
                            ? '加载中…'
                            : showingSearch
                                ? '加载更多搜索结果'
                                : '加载更早记录'
                    }}
                </button>
            </footer>
        </section>
    </dialog>
</template>
