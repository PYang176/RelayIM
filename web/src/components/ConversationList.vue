<script setup lang="ts">
import type {Conversation} from '@/types/api'

const props = defineProps<{
    conversations: Conversation[]
    activeId: string
    loading: boolean
    loadingMore: boolean
    hasMore: boolean
}>()

const emit = defineEmits<{
    select: [conversationId: string]
    createGroup: []
    openFriends: []
    loadMore: []
}>()

function title(conversation: Conversation) {
    return conversation.title || conversation.name || '私聊'
}

function summary(conversation: Conversation) {
    if (conversation.conversation_type === 2) {
        return `群聊 · ${conversation.member_count} 人`
    }
    return '私聊'
}
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

        <p v-if="loading" class="panel-hint" role="status" aria-busy="true">
            加载会话中…
        </p>
        <p v-else-if="conversations.length === 0" class="panel-hint" role="status">
            还没有会话。打开好友列表，选择好友开始私聊或创建群聊。
        </p>

        <div class="conversation-list">
            <button
                    v-for="conversation in conversations"
                    :key="conversation.id"
                    :class="{ active: conversation.id === activeId }"
                    :aria-current="conversation.id === activeId ? 'true' : undefined"
                    class="conversation-item"
                    type="button"
                    @click="emit('select', conversation.id)"
            >
        <span class="conversation-avatar">
          {{ title(conversation).slice(0, 1) }}
        </span>
                <span class="conversation-copy">
          <strong>{{ title(conversation) }}</strong>
          <small>
            {{ summary(conversation) }}
            · 序号 {{ conversation.last_seq }}
          </small>
        </span>
                <span v-if="conversation.unread_count > 0" class="unread-badge">
          {{ conversation.unread_count > 99 ? '99+' : conversation.unread_count }}
        </span>
            </button>
            <button
                    v-if="hasMore"
                    :disabled="loadingMore"
                    class="conversation-load-more"
                    type="button"
                    @click="emit('loadMore')"
            >
                {{ loadingMore ? '加载中…' : '加载更多会话' }}
            </button>
        </div>
    </aside>
</template>
