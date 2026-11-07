<script setup lang="ts">
import {shallowRef} from 'vue'

import type {Conversation} from '@/types/api'

const props = defineProps<{
    conversations: Conversation[]
    activeId: string
    loading: boolean
}>()

const emit = defineEmits<{
    select: [conversationId: string]
    createDirect: [peerUserId: string]
}>()

const peerUserId = shallowRef('')
const showCreate = shallowRef(false)

function title(conversation: Conversation) {
    return conversation.name || `私聊 · ${conversation.id.slice(-6)}`
}

function create() {
    const value = peerUserId.value.trim()
    if (!value) return
    emit('createDirect', value)
    peerUserId.value = ''
    showCreate.value = false
}
</script>

<template>
    <aside class="conversation-panel">
        <div class="conversation-heading">
            <div>
                <h2>消息</h2>
            </div>
            <button
                    class="icon-button"
                    :aria-expanded="showCreate"
                    aria-controls="create-direct-form"
                    aria-label="发起私聊"
                    type="button"
                    @click="showCreate = !showCreate"
            >
                +
            </button>
        </div>

        <form
                v-if="showCreate"
                id="create-direct-form"
                class="create-direct"
                @submit.prevent="create"
        >
            <label class="sr-only" for="peer-user-id">对方用户 ID</label>
            <input
                    id="peer-user-id"
                    v-model="peerUserId"
                    aria-label="对方用户 ID"
                    inputmode="numeric"
                    placeholder="对方用户 ID"
                    required
            />
            <button type="submit">创建</button>
        </form>

        <p v-if="loading" class="panel-hint" role="status" aria-busy="true">
            加载会话中…
        </p>
        <p v-else-if="conversations.length === 0" class="panel-hint" role="status">
            还没有会话。使用上方“发起私聊”输入对方用户 ID。
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
            {{ conversation.conversation_type === 2 ? '群聊' : '私聊' }}
            · 序号 {{ conversation.last_seq }}
          </small>
        </span>
                <span v-if="conversation.unread_count > 0" class="unread-badge">
          {{ conversation.unread_count > 99 ? '99+' : conversation.unread_count }}
        </span>
            </button>
        </div>
    </aside>
</template>
