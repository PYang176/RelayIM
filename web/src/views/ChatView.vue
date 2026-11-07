<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'

import ConversationList from '@/components/ConversationList.vue'
import MessageComposer from '@/components/MessageComposer.vue'
import MessageList from '@/components/MessageList.vue'
import { useAuthStore } from '@/stores/auth'
import { useChatStore } from '@/stores/chat'

const auth = useAuthStore()
const chat = useChatStore()
const router = useRouter()
const messageList = ref<InstanceType<typeof MessageList> | null>(null)

const connectionLabel = computed(() => {
  const labels = {
    connected: '实时连接正常',
    connecting: '正在连接',
    reconnecting: '正在重连',
    disconnected: '连接已断开',
  }
  return labels[chat.connectionState]
})

async function createDirect(peerUserId: string) {
  try {
    await chat.createDirect(peerUserId)
    messageList.value?.scrollToBottom()
  } catch (cause) {
    chat.$patch({
      error: cause instanceof Error ? cause.message : '创建私聊失败',
    })
  }
}

async function selectConversation(conversationId: string) {
  await chat.selectConversation(conversationId)
  messageList.value?.scrollToBottom()
}

async function sendMessage(content: string) {
  try {
    await chat.sendMessage(content)
    messageList.value?.scrollToBottom()
  } catch (cause) {
    chat.$patch({
      error: cause instanceof Error ? cause.message : '发送消息失败',
    })
  }
}

function markVisibleConversationRead() {
  if (document.visibilityState === 'visible') {
    void chat.markRead()
  }
}

async function logout() {
  chat.reset()
  auth.logout()
  await router.replace('/login')
}

onMounted(async () => {
  document.addEventListener('visibilitychange', markVisibleConversationRead)
  try {
    await auth.restoreUser()
  } catch {
    auth.logout()
    await router.replace('/login')
    return
  }
  await chat.connectWebSocket()
})

onBeforeUnmount(() => {
  document.removeEventListener('visibilitychange', markVisibleConversationRead)
  chat.disconnectWebSocket()
})
</script>

<template>
  <main class="chat-page">
    <header class="app-header">
      <div class="app-brand">
        <span class="brand-dot">G</span>
        <div>
          <strong>GinChat</strong>
          <small :class="['connection-state', chat.connectionState]">
            {{ connectionLabel }}
          </small>
        </div>
      </div>
      <div class="current-user">
        <span>{{ auth.user?.nickname || auth.user?.username }}</span>
        <button class="ghost-button" type="button" @click="logout">退出</button>
      </div>
    </header>

    <p v-if="chat.error" class="global-error" @click="chat.$patch({ error: '' })">
      {{ chat.error }}
    </p>

    <section class="chat-layout">
      <ConversationList
        :active-id="chat.activeConversationId"
        :conversations="chat.conversations"
        :loading="chat.loadingConversations"
        @create-direct="createDirect"
        @select="selectConversation"
      />

      <section class="conversation-main">
        <template v-if="chat.activeConversation">
          <header class="conversation-header">
            <div>
              <h1>{{ chat.conversationTitle(chat.activeConversation) }}</h1>
              <p>
                最后序号 {{ chat.activeConversation.last_seq }}
                <span v-if="chat.activeReadStates.length">
                  · 对方已读 {{ chat.activeReadStates[0].last_read_seq }}
                </span>
              </p>
            </div>
            <span class="conversation-id">
              ID {{ chat.activeConversation.id }}
            </span>
          </header>

          <MessageList
            ref="messageList"
            :current-user-id="auth.user?.id ?? ''"
            :has-more="Boolean(chat.hasMoreByConversation[chat.activeConversationId])"
            :loading="chat.loadingMessages"
            :messages="chat.activeMessages"
            @load-older="chat.loadOlderMessages"
          />
          <MessageComposer
            :disabled="chat.connectionState !== 'connected'"
            @send="sendMessage"
          />
        </template>

        <div v-else class="empty-workspace">
          <span class="empty-logo">G</span>
          <h1>选择一个会话</h1>
          <p>或者使用左侧“+”输入对方用户 ID 创建私聊。</p>
        </div>
      </section>
    </section>
  </main>
</template>
