import { defineStore } from 'pinia'
import { computed, markRaw, ref } from 'vue'

import * as authApi from '@/api/auth'
import * as conversationApi from '@/api/conversations'
import * as messageApi from '@/api/messages'
import { useAuthStore } from '@/stores/auth'
import type {
  Conversation,
  DeliverySummary,
  Message,
  ReadState,
  WsMessage,
} from '@/types/api'

export type ConnectionState = 'disconnected' | 'connecting' | 'connected' | 'reconnecting'

export interface ChatMessage extends Message {
  delivery?: DeliverySummary
  outgoing?: boolean
}

const DEVICE_KEY = 'ginchat.device_id'

export const useChatStore = defineStore('chat', () => {
  const conversations = ref<Conversation[]>([])
  const messagesByConversation = ref<Record<string, ChatMessage[]>>({})
  const activeConversationId = ref('')
  const readStates = ref<ReadState[]>([])
  const connectionState = ref<ConnectionState>('disconnected')
  const loadingConversations = ref(false)
  const loadingMessages = ref(false)
  const hasMoreByConversation = ref<Record<string, boolean>>({})
  const error = ref('')

  let socket: WebSocket | null = null
  let reconnectTimer: number | undefined
  let reconnectAttempt = 0
  let intentionallyClosed = false
  let connecting = false

  const activeConversation = computed(() =>
    conversations.value.find((item) => item.id === activeConversationId.value),
  )
  const activeMessages = computed(
    () => messagesByConversation.value[activeConversationId.value] ?? [],
  )
  const activeReadStates = computed(() =>
    readStates.value.filter((item) => item.user_id !== useAuthStore().user?.id),
  )

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
      message.sender_id !== useAuthStore().user?.id &&
      message.conversation_id !== activeConversationId.value
    if (isIncoming || document.visibilityState !== 'visible') {
      conversation.unread_count += 1
    }
  }

  async function loadConversations() {
    const auth = useAuthStore()
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
    const auth = useAuthStore()
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
    const auth = useAuthStore()
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

    const auth = useAuthStore()
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
    const auth = useAuthStore()
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

    const auth = useAuthStore()
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
    const auth = useAuthStore()
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
    const auth = useAuthStore()
    try {
      readStates.value = await messageApi.listReadStates(auth.token, conversationId)
    } catch {
      readStates.value = []
    }
  }

  function sendSocket(payload: WsMessage) {
    if (!socket || socket.readyState !== WebSocket.OPEN) return false
    socket.send(JSON.stringify(payload))
    return true
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

  async function connectWebSocket() {
    if (connecting) return
    const auth = useAuthStore()
    if (!auth.token) return
    connecting = true
    intentionallyClosed = false
    clearTimeout(reconnectTimer)
    connectionState.value = reconnectAttempt > 0 ? 'reconnecting' : 'connecting'

    try {
      const deviceId = localStorage.getItem(DEVICE_KEY) ?? ''
      const ticket = await authApi.issueWsTicket(auth.token, deviceId)
      localStorage.setItem(DEVICE_KEY, ticket.device_id)

      if (socket) {
        intentionallyClosed = true
        socket.close()
        intentionallyClosed = false
      }
      const scheme = window.location.protocol === 'https:' ? 'wss' : 'ws'
      const ws = new WebSocket(
        `${scheme}://${window.location.host}/ws?ticket=${encodeURIComponent(ticket.ticket)}`,
      )
      socket = markRaw(ws)

      ws.onopen = () => {
        if (socket !== ws) return
        reconnectAttempt = 0
        connectionState.value = 'connected'
        void loadConversations().then(async () => {
          for (const conversation of conversations.value) {
            await syncConversationAfterReconnect(conversation)
          }
          await markRead()
        })
      }
      ws.onmessage = (event) => {
        if (socket !== ws) return
        try {
          handleSocketMessage(JSON.parse(String(event.data)) as WsMessage)
        } catch {
          setError('收到无法解析的 WebSocket 消息')
        }
      }
      ws.onerror = () => {
        if (socket === ws) {
          connectionState.value = 'disconnected'
        }
      }
      ws.onclose = () => {
        if (socket !== ws) return
        socket = null
        connectionState.value = 'disconnected'
        if (!intentionallyClosed && auth.token) {
          scheduleReconnect()
        }
      }
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : '建立实时连接失败')
      connectionState.value = 'disconnected'
      scheduleReconnect()
    } finally {
      connecting = false
    }
  }

  function scheduleReconnect() {
    clearTimeout(reconnectTimer)
    reconnectAttempt += 1
    const delay = Math.min(1000 * 2 ** (reconnectAttempt - 1), 30_000)
    connectionState.value = 'reconnecting'
    reconnectTimer = window.setTimeout(() => {
      void connectWebSocket()
    }, delay)
  }

  function disconnectWebSocket() {
    intentionallyClosed = true
    clearTimeout(reconnectTimer)
    socket?.close()
    socket = null
    connectionState.value = 'disconnected'
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
    reset,
  }
})
