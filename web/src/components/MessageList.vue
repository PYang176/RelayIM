<script setup lang="ts">
import {nextTick, shallowRef, useTemplateRef, watch} from 'vue'

import type {ChatMessage} from '@/stores/chat'

const props = defineProps<{
    messages: ChatMessage[]
    currentUserId: string
    currentUserRole: number
    conversationType: number
    loading: boolean
    hasMore: boolean
}>()

const emit = defineEmits<{
    loadOlder: []
    recall: [messageId: string]
}>()

const scroller = useTemplateRef<HTMLElement>('scroller')
const pendingRecallId = shallowRef('')
const senderRecallWindow = 2 * 60 * 1000
const managerRecallWindow = 24 * 60 * 60 * 1000

// formatTime 格式化消息列表中的时间
function formatTime(value: string) {
    return new Intl.DateTimeFormat('zh-CN', {
        hour: '2-digit',
        minute: '2-digit',
    }).format(new Date(value))
}

// isOutgoing 判断消息是否由当前用户发送
function isOutgoing(message: ChatMessage) {
    return message.outgoing || message.sender_id === props.currentUserId
}

// canRecall 判断当前消息是否满足撤回条件
function canRecall(message: ChatMessage) {
    if (message.recalled || !props.currentUserId) return false
    const age = Date.now() - new Date(message.created_at).getTime()
    if (message.sender_id === props.currentUserId && age <= senderRecallWindow) {
        return true
    }
    return props.conversationType === 2 &&
        props.currentUserRole >= 2 &&
        age <= managerRecallWindow
}

// recall 确认并请求撤回指定消息
function recall(message: ChatMessage) {
    if (!canRecall(message)) return
    if (pendingRecallId.value !== message.message_id) {
        pendingRecallId.value = message.message_id
        return
    }
    pendingRecallId.value = ''
    emit('recall', message.message_id)
}

// scrollToBottom 滚动消息容器到底部
function scrollToBottom() {
    void nextTick(() => {
        if (scroller.value) {
            scroller.value.scrollTop = scroller.value.scrollHeight
        }
    })
}

watch(
    () => props.messages.length,
    (next, previous) => {
        if (next > previous) {
            scrollToBottom()
        }
    },
)

defineExpose({scrollToBottom})
</script>

<template>
    <div
            id="message-log"
            ref="scroller"
            class="message-scroller"
            role="log"
            aria-live="polite"
            aria-relevant="additions text"
            aria-label="当前会话消息"
    >
        <button
                v-if="hasMore"
                class="load-older"
                type="button"
                @click="emit('loadOlder')"
        >
            加载更早消息
        </button>

        <div v-if="loading" class="message-skeleton" role="status" aria-busy="true">
            <span/>
            <span/>
            <span/>
            <span class="sr-only">加载消息中</span>
        </div>
        <p v-else-if="messages.length === 0" class="empty-chat" role="status">
            <span>还没有消息</span>
            发送第一条消息开始聊天
        </p>

        <article
                v-for="message in messages"
                :key="message.message_id"
                :class="[
                    'message-row',
                    {
                        outgoing: isOutgoing(message),
                        recalled: message.recalled,
                    },
                ]"
        >
            <span class="message-seq" aria-label="消息序号">{{ message.seq }}</span>
            <div class="message-bubble">
                <p v-if="message.recalled" class="message-recalled" role="status">
                    该消息已撤回
                </p>
                <template v-else>
                    <figure
                            v-if="message.message_type === 2 && message.attachment_url"
                            class="message-image"
                    >
                        <a
                                :href="message.attachment_url"
                                rel="noopener noreferrer"
                                target="_blank"
                        >
                            <img
                                    :alt="message.attachment_name || '聊天图片'"
                                    :src="message.attachment_url"
                                    loading="lazy"
                            />
                        </a>
                    </figure>
                    <a
                            v-else-if="message.message_type === 3 && message.attachment_url"
                            class="message-file"
                            :href="message.attachment_url"
                            rel="noopener noreferrer"
                            target="_blank"
                    >
                        <span class="message-file-mark">FILE</span>
                        <span class="message-file-copy">
                            <strong>{{ message.attachment_name || '聊天附件' }}</strong>
                            <small>点击下载文件</small>
                        </span>
                    </a>
                    <p v-if="message.content">{{ message.content }}</p>
                    <p
                            v-else-if="
                                message.message_type !== 1 &&
                                !message.attachment_url
                            "
                            class="attachment-loading"
                    >
                        附件地址加载中…
                    </p>
                </template>
                <footer>
                    <time>{{ formatTime(message.created_at) }}</time>
                    <span
                            v-if="isOutgoing(message) && !message.recalled"
                            class="delivery-state"
                    >
            <template v-if="message.delivery">
              已送达 {{ message.delivery.delivered_devices }} 台设备
            </template>
            <template v-else>已发送</template>
          </span>
                    <button
                            v-if="canRecall(message)"
                            class="text-button message-recall-button"
                            type="button"
                            @click="recall(message)"
                    >
                        {{
                            pendingRecallId === message.message_id
                                ? '确认撤回'
                                : '撤回'
                        }}
                    </button>
                </footer>
            </div>
        </article>
    </div>
</template>
