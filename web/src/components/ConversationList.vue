<script setup lang="ts">
import {onBeforeUnmount, shallowRef, watch} from 'vue'

import type {Conversation} from '@/types/api'

const props = defineProps<{
    conversations: Conversation[]
    activeId: string
    loading: boolean
    loadingMore: boolean
    hasMore: boolean
    searchQuery: string
    searching: boolean
}>()

const emit = defineEmits<{
    select: [conversationId: string]
    createGroup: []
    openFriends: []
    loadMore: []
    search: [query: string]
    clearSearch: []
    togglePin: [conversationId: string, pinned: boolean]
}>()

const searchText = shallowRef(props.searchQuery)
let searchTimer = 0

watch(
    () => props.searchQuery,
    (query) => {
        searchText.value = query
    },
)

// scheduleSearch 延迟触发会话搜索并重置上一次计时器
function scheduleSearch() {
    window.clearTimeout(searchTimer)
    const query = searchText.value.trim()
    if (!query) {
        emit('clearSearch')
        return
    }
    searchTimer = window.setTimeout(() => {
        emit('search', query)
    }, 300)
}

// clearSearch 清空会话搜索词并恢复完整列表
function clearSearch() {
    window.clearTimeout(searchTimer)
    searchText.value = ''
    emit('clearSearch')
}

// title 计算会话在列表中的展示标题
function title(conversation: Conversation) {
    return conversation.title || conversation.name || '私聊'
}

// summary 计算会话最近消息摘要
function summary(conversation: Conversation) {
    if (conversation.conversation_type === 2) {
        return `群聊 · ${conversation.member_count} 人`
    }
    return '私聊'
}

onBeforeUnmount(() => {
    window.clearTimeout(searchTimer)
})
</script>

<template>
    <aside class="conversation-panel">
        <div class="conversation-heading">
            <div>
                <h2>消息</h2>
            </div>
            <div class="conversation-heading-actions">
                <button
                        class="icon-button group-button"
                        aria-label="创建群聊"
                        type="button"
                        @click="emit('createGroup')"
                >
                    群
                </button>
                <button
                        class="icon-button"
                        aria-label="从好友发起私聊"
                        type="button"
                        @click="emit('openFriends')"
                >
                    +
                </button>
            </div>
        </div>

        <label class="conversation-search">
            <span class="sr-only">搜索会话</span>
            <input
                    v-model="searchText"
                    :aria-busy="searching"
                    autocomplete="off"
                    placeholder="搜索会话"
                    type="search"
                    @input="scheduleSearch"
            />
            <button
                    v-if="searchText"
                    aria-label="清除会话搜索"
                    type="button"
                    @click="clearSearch"
            >
                ×
            </button>
        </label>

        <p v-if="loading" class="panel-hint" role="status" aria-busy="true">
            加载会话中…
        </p>
        <p v-else-if="searching" class="panel-hint" role="status" aria-busy="true">
            搜索会话中…
        </p>
        <p
                v-else-if="searchQuery && conversations.length === 0"
                class="panel-hint"
                role="status"
        >
            没有匹配“{{ searchQuery }}”的会话。
        </p>
        <p
                v-else-if="!searchQuery && conversations.length === 0"
                class="panel-hint"
                role="status"
        >
            还没有会话。打开好友列表，选择好友开始私聊或创建群聊。
        </p>

        <div class="conversation-list">
            <div
                    v-for="conversation in conversations"
                    :key="conversation.id"
                    :class="{
                        active: conversation.id === activeId,
                        pinned: conversation.pinned,
                    }"
                    class="conversation-item"
            >
                <button
                        :aria-current="conversation.id === activeId ? 'true' : undefined"
                        class="conversation-open"
                        type="button"
                        @click="emit('select', conversation.id)"
                >
                    <span class="conversation-avatar">
                        {{ title(conversation).slice(0, 1) }}
                    </span>
                    <span class="conversation-copy">
                        <strong>{{ title(conversation) }}</strong>
                        <small>
                            <span v-if="conversation.pinned">置顶 · </span>
                            {{ summary(conversation) }}
                            · 序号 {{ conversation.last_seq }}
                        </small>
                    </span>
                    <span
                            v-if="conversation.unread_count > 0"
                            class="unread-badge"
                    >
                        {{
                            conversation.unread_count > 99
                                ? '99+'
                                : conversation.unread_count
                        }}
                    </span>
                </button>
                <button
                        :aria-label="
                            conversation.pinned
                                ? `取消置顶${title(conversation)}`
                                : `置顶${title(conversation)}`
                        "
                        :aria-pressed="conversation.pinned"
                        class="conversation-pin"
                        type="button"
                        @click="
                            emit(
                                'togglePin',
                                conversation.id,
                                !conversation.pinned,
                            )
                        "
                >
                    {{ conversation.pinned ? '取消' : '置顶' }}
                </button>
            </div>
            <button
                    v-if="hasMore"
                    :disabled="loadingMore"
                    class="conversation-load-more"
                    type="button"
                    @click="emit('loadMore')"
            >
                {{
                    loadingMore
                        ? '加载中…'
                        : searchQuery
                            ? '加载更多搜索结果'
                            : '加载更多会话'
                }}
            </button>
        </div>
    </aside>
</template>
