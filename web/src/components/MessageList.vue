<script setup lang="ts">
import { nextTick, useTemplateRef, watch } from 'vue'

import type { ChatMessage } from '@/stores/chat'

const props = defineProps<{
  messages: ChatMessage[]
  currentUserId: string
  loading: boolean
  hasMore: boolean
}>()

const emit = defineEmits<{
  loadOlder: []
}>()

const scroller = useTemplateRef<HTMLElement>('scroller')

function formatTime(value: string) {
  return new Intl.DateTimeFormat('zh-CN', {
    hour: '2-digit',
    minute: '2-digit',
  }).format(new Date(value))
}

function isOutgoing(message: ChatMessage) {
  return message.outgoing || message.sender_id === props.currentUserId
}

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

defineExpose({ scrollToBottom })
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
      <span />
      <span />
      <span />
      <span class="sr-only">加载消息中</span>
    </div>
    <p v-else-if="messages.length === 0" class="empty-chat" role="status">
      <span>还没有消息</span>
      发送第一条消息开始聊天
    </p>

    <article
      v-for="message in messages"
      :key="message.message_id"
      :class="['message-row', { outgoing: isOutgoing(message) }]"
    >
      <span class="message-seq" aria-label="消息序号">{{ message.seq }}</span>
      <div class="message-bubble">
        <p>{{ message.content }}</p>
        <footer>
          <time>{{ formatTime(message.created_at) }}</time>
          <span v-if="isOutgoing(message)" class="delivery-state">
            <template v-if="message.delivery">
              已送达 {{ message.delivery.delivered_devices }} 台设备
            </template>
            <template v-else>已发送</template>
          </span>
        </footer>
      </div>
    </article>
  </div>
</template>
