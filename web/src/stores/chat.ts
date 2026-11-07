import { defineStore } from 'pinia'
import { computed, ref, shallowRef } from 'vue'

import * as conversationApi from '@/api/conversations'
import * as messageApi from '@/api/messages'
import { useChatSocket } from '@/composables/useChatSocket'
import { useAuthStore } from '@/stores/auth'
import type {
  Conversation,
  DeliverySummary,
  Message,
  ReadState,
  WsMessage,
} from '@/types/api'

export interface ChatMessage extends Message {
  delivery?: DeliverySummary
  outgoing?: boolean
}

export const useChatStore = defineStore('chat', () => {
  const auth = useAuthStore()
  const conversations = ref<Conversation[]>([])
  const messagesByConversation = ref<Record<string, ChatMessage[]>>({})
  const activeConversationId = shallowRef('')
  const readStates = ref<ReadState[]>([])
  const loadingConversations = shallowRef(false)
  const loadingMessages = shallowRef(false)
  const hasMoreByConversation = ref<Record<string, boolean>>({})
  const error = shallowRef('')

  const activeConversation = computed(() =>
    conversations.value.find((item) => item.id === activeConversationId.value),
  )
  const activeMessages = computed(
    () => messagesByConversation.value[activeConversationId.value] ?? [],
  )
  const activeReadStates = computed(() =>
    readStates.value.filter((item) => item.user_id !== auth.user?.id),
  )

  const realtime = useChatSocket({
    getToken: () => auth.token,
    onConnected: handleSocketConnected,
    onMessage: handleSocketMessage,
    onError: setError,
  })
  const connectionState = realtime.connectionState

  function setError(message: string) {
    error.value = message
  }

  function clearError() {
    error.value = ''
  }

  function conversationTitle(conversation: Conversation): string {
    if (conversation.name) return conversation.name
    return `私聊 · ${conversation.id.slice(-6)}`
  }

  function upsertConversation(conversation: Conversation) {
    const index = conversations.value.findIndex((item) => item.id === conversation.id)
    if (index >= 0) {
      conversations.value[index] = conversation
      return
    }
    conversations.value.unshift(conversation)
  }

  function upsertMessage(conversationId: string, message: ChatMessage) {
    const current = messagesByConversation.value[conversationId] ?? []
    const index = current.findIndex((item) => item.message_id === message.message_id)
    const next = [...current]
    if (index >= 0) {
      next[index] = { ...next[index], ...message }
    } else {
      next.push(message)
    }
    next.sort((left, right) => left.seq - right.seq)
    messagesByConversation.value = {
      ...messagesByConversation.value,
      [conversationId]: next,
    }
  }

  function updateConversationFromMessage(message: Message) {
    const conversation = conversations.value.find(
      (item) => item.id === message.conversation_id,
    )
    if (!conversation) {
      void loadConversations()
      return
    }
    conversation.last_seq = Math.max(conversation.last_seq, message.seq)
    const isIncoming =
      message.sender_id !== auth.user?.id &&
      message.conversation_id !== activeConversationId.value
    if (isIncoming || document.visibilityState !== 'visible') {
      conversation.unread_count += 1
    }
  }

  async function loadConversations() {
    loadingConversations.value = true
    try {
      conversations.value = await conversationApi.listConversations(auth.token)
      if (!activeConversationId.value && conversations.value.length > 0) {
        await selectConversation(conversations.value[0].id)
      }
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : '加载会话失败')
    } finally {
      loadingConversations.value = false
    }
  }

  async function createDirect(peerUserId: string) {
    clearError()
    const conversation = await conversationApi.createDirect(
      auth.token,
      peerUserId.trim(),
    )
    upsertConversation(conversation)
    await selectConversation(conversation.id)
  }

  async function selectConversation(conversationId: string) {
    activeConversationId.value = conversationId
    readStates.value = []
    if (!messagesByConversation.value[conversationId]) {
      await loadLatestMessages(conversationId)
    }
    await markRead()
    await refreshReadStates()
  }

  async function loadLatestMessages(conversationId: string) {
    loadingMessages.value = true
    clearError()
    try {
      const page = await messageApi.listMessages(auth.token, conversationId, {
        limit: 40,
      })
      const merged = new Map<string, ChatMessage>()
      for (const message of messagesByConversation.value[conversationId] ?? []) {
        merged.set(message.message_id, message)
      }
      for (const message of page.messages) {
        merged.set(message.message_id, message)
      }
      messagesByConversation.value = {
        ...messagesByConversation.value,
        [conversationId]: [...merged.values()].sort(
          (left, right) => left.seq - right.seq,
        ),
      }
      hasMoreByConversation.value = {
        ...hasMoreByConversation.value,
        [conversationId]: page.has_more,
      }
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : '加载消息失败')
    } finally {
      loadingMessages.value = false
    }
  }

  async function loadOlderMessages() {
    const conversationId = activeConversationId.value
    const messages = messagesByConversation.value[conversationId] ?? []
    if (!conversationId || messages.length === 0) return
    if (!hasMoreByConversation.value[conversationId]) return

    const page = await messageApi.listMessages(auth.token, conversationId, {
      beforeSeq: messages[0].seq,
      limit: 30,
    })
    const merged = new Map<string, ChatMessage>()
    for (const message of [...page.messages, ...messages]) {
      merged.set(message.message_id, message)
    }
    messagesByConversation.value = {
      ...messagesByConversation.value,
      [conversationId]: [...merged.values()].sort(
        (left, right) => left.seq - right.seq,
      ),
    }
    hasMoreByConversation.value = {
      ...hasMoreByConversation.value,
      [conversationId]: page.has_more,
    }
  }

  async function syncConversationAfterReconnect(conversation: Conversation) {
    const localMessages = messagesByConversation.value[conversation.id] ?? []
    const lastLocalSeq =
      localMessages.length > 0 ? localMessages[localMessages.length - 1].seq : 0
    const afterSeq = Math.max(lastLocalSeq, conversation.last_read_seq)
    const page = await messageApi.listMessages(auth.token, conversation.id, {
      afterSeq: afterSeq || undefined,
      limit: 100,
    })
    for (const message of page.messages) {
      upsertMessage(conversation.id, message)
      if (message.conversation_id === activeConversationId.value) {
        sendAck(message.message_id)
      }
    }
    if (page.has_more) {
      await syncConversationAfterReconnect(conversation)
    }
  }

  async function sendMessage(content: string) {
    const conversationId = activeConversationId.value
    const trimmed = content.trim()
    if (!conversationId || !trimmed) return

    clearError()
    const response = await messageApi.sendText(
      auth.token,
      conversationId,
      crypto.randomUUID(),
      trimmed,
    )
    upsertMessage(conversationId, {
      message_id: response.message_id,
      conversation_id: response.conversation_id,
      sender_id: auth.user?.id ?? '',
      seq: response.seq,
      message_type: 1,
      content: trimmed,
      created_at: response.created_at,
      outgoing: true,
    })
    const conversation = conversations.value.find((item) => item.id === conversationId)
    if (conversation) {
      conversation.last_seq = Math.max(conversation.last_seq, response.seq)
    }
    window.setTimeout(() => {
      void refreshDelivery(conversationId, response.message_id)
    }, 1200)
  }

  async function refreshDelivery(conversationId: string, messageId: string) {
    try {
      const delivery = await messageApi.getDeliverySummary(
        auth.token,
        conversationId,
        messageId,
      )
      const messages = messagesByConversation.value[conversationId] ?? []
      const message = messages.find((item) => item.message_id === messageId)
      if (message) {
        message.delivery = delivery
      }
    } catch {
      // 送达状态刷新失败不回滚已经成功发送的消息。
    }
  }

  async function refreshReadStates() {
    const conversationId = activeConversationId.value
    if (!conversationId) return
    try {
      readStates.value = await messageApi.listReadStates(auth.token, conversationId)
    } catch {
      readStates.value = []
    }
  }

  function sendSocket(payload: WsMessage) {
    return realtime.send(payload)
  }

  function sendAck(messageId: string) {
    sendSocket({
      action: 'ack',
      message_id: messageId,
    })
  }

  async function markRead() {
    const conversation = activeConversation.value
    if (!conversation) return
    const messages = messagesByConversation.value[conversation.id] ?? []
    const seq = messages.length > 0 ? messages[messages.length - 1].seq : conversation.last_seq
    if (!seq) return
    if (
      sendSocket({
        action: 'read',
        message_id: '',
        conversation_id: conversation.id,
        seq,
      })
    ) {
      conversation.last_read_seq = Math.max(conversation.last_read_seq, seq)
      conversation.unread_count = 0
      window.setTimeout(() => {
        void refreshReadStates()
      }, 300)
    }
  }

  function handleSocketMessage(payload: WsMessage) {
    if (
      payload.action !== 'message' ||
      !payload.message_id ||
      !payload.conversation_id ||
      !payload.sender_id ||
      !payload.seq
    ) {
      return
    }
    const message: ChatMessage = {
      message_id: payload.message_id,
      conversation_id: payload.conversation_id,
      sender_id: payload.sender_id,
      seq: payload.seq,
      message_type: payload.msg_type ?? 1,
      content: payload.content ?? '',
      created_at: new Date().toISOString(),
    }
    upsertMessage(payload.conversation_id, message)
    updateConversationFromMessage(message)
    sendAck(message.message_id)
    if (
      payload.conversation_id === activeConversationId.value &&
      document.visibilityState === 'visible'
    ) {
      void markRead()
    }
  }

  async function handleSocketConnected() {
    await loadConversations()
    for (const conversation of conversations.value) {
      await syncConversationAfterReconnect(conversation)
    }
    await markRead()
  }

  function connectWebSocket() {
    return realtime.connect()
  }

  function disconnectWebSocket() {
    realtime.disconnect()
  }

  function reset() {
    disconnectWebSocket()
    conversations.value = []
    messagesByConversation.value = {}
    activeConversationId.value = ''
    readStates.value = []
    error.value = ''
  }

  return {
    conversations,
    messagesByConversation,
    activeConversationId,
    activeConversation,
    activeMessages,
    activeReadStates,
    connectionState,
    loadingConversations,
    loadingMessages,
    hasMoreByConversation,
    error,
    conversationTitle,
    loadConversations,
    createDirect,
    selectConversation,
    loadOlderMessages,
    sendMessage,
    connectWebSocket,
    disconnectWebSocket,
    markRead,
    reportError: setError,
    clearError,
    reset,
  }
})
