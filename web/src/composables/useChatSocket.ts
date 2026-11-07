import { readonly, shallowRef } from 'vue'

import { issueWsTicket } from '@/api/auth'
import type { WsMessage } from '@/types/api'

export type ConnectionState =
  | 'disconnected'
  | 'connecting'
  | 'connected'
  | 'reconnecting'

interface ChatSocketOptions {
  getToken: () => string
  onConnected: () => Promise<void> | void
  onMessage: (message: WsMessage) => void
  onError: (message: string) => void
}

const DEVICE_KEY = 'ginchat.device_id'

/**
 * 管理一次性 Ticket、WebSocket 生命周期和指数退避重连。
 * 领域消息处理、会话状态与 DOM 展示不在这里实现。
 */
export function useChatSocket(options: ChatSocketOptions) {
  const connectionState = shallowRef<ConnectionState>('disconnected')

  let socket: WebSocket | null = null
  let reconnectTimer: number | undefined
  let reconnectAttempt = 0
  let intentionallyClosed = false
  let connecting = false

  async function connect() {
    if (connecting) return
    const token = options.getToken()
    if (!token) return

    connecting = true
    intentionallyClosed = false
    clearTimeout(reconnectTimer)
    connectionState.value = reconnectAttempt > 0 ? 'reconnecting' : 'connecting'

    try {
      const savedDeviceID = localStorage.getItem(DEVICE_KEY) ?? ''
      const ticket = await issueWsTicket(token, savedDeviceID)
      localStorage.setItem(DEVICE_KEY, ticket.device_id)

      if (socket) {
        const previous = socket
        socket = null
        previous.close()
      }

      const scheme = window.location.protocol === 'https:' ? 'wss' : 'ws'
      const nextSocket = new WebSocket(
        `${scheme}://${window.location.host}/ws?ticket=${encodeURIComponent(ticket.ticket)}`,
      )
      socket = nextSocket

      nextSocket.onopen = () => {
        if (socket !== nextSocket) return
        reconnectAttempt = 0
        connectionState.value = 'connected'
        Promise.resolve(options.onConnected()).catch((cause: unknown) => {
          options.onError(
            cause instanceof Error ? cause.message : '实时连接初始化失败',
          )
        })
      }
      nextSocket.onmessage = (event) => {
        if (socket !== nextSocket) return
        try {
          options.onMessage(JSON.parse(String(event.data)) as WsMessage)
        } catch {
          options.onError('收到无法解析的 WebSocket 消息')
        }
      }
      nextSocket.onerror = () => {
        if (socket === nextSocket) {
          connectionState.value = 'disconnected'
        }
      }
      nextSocket.onclose = () => {
        if (socket !== nextSocket) return
        socket = null
        connectionState.value = 'disconnected'
        if (!intentionallyClosed && options.getToken()) {
          scheduleReconnect()
        }
      }
    } catch (cause) {
      options.onError(cause instanceof Error ? cause.message : '建立实时连接失败')
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
      void connect()
    }, delay)
  }

  function send(payload: WsMessage): boolean {
    if (!socket || socket.readyState !== WebSocket.OPEN) return false
    socket.send(JSON.stringify(payload))
    return true
  }

  function disconnect() {
    intentionallyClosed = true
    clearTimeout(reconnectTimer)
    if (socket) {
      socket.close()
      socket = null
    }
    connectionState.value = 'disconnected'
  }

  return {
    connectionState: readonly(connectionState),
    connect,
    disconnect,
    send,
  }
}
