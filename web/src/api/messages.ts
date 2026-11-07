import { request } from './http'
import type {
  DeliverySummary,
  MessagePage,
  ReadState,
  SendMessageResponse,
} from '@/types/api'

export function listMessages(
  token: string,
  conversationId: string,
  query: { afterSeq?: number; beforeSeq?: number; limit?: number },
): Promise<MessagePage> {
  const params = new URLSearchParams()
  if (query.afterSeq) params.set('after_seq', String(query.afterSeq))
  if (query.beforeSeq) params.set('before_seq', String(query.beforeSeq))
  params.set('limit', String(query.limit ?? 30))
  return request(`/conversations/${conversationId}/messages?${params}`, {}, token)
}

export function sendText(
  token: string,
  conversationId: string,
  clientMessageId: string,
  content: string,
): Promise<SendMessageResponse> {
  return request(
    `/conversations/${conversationId}/messages`,
    {
      method: 'POST',
      body: JSON.stringify({
        client_message_id: clientMessageId,
        content,
      }),
    },
    token,
  )
}

export async function listReadStates(
  token: string,
  conversationId: string,
): Promise<ReadState[]> {
  const response = await request<{ read_states: ReadState[] }>(
    `/conversations/${conversationId}/read-states`,
    {},
    token,
  )
  return response.read_states
}

export function getDeliverySummary(
  token: string,
  conversationId: string,
  messageId: string,
): Promise<DeliverySummary> {
  return request(
    `/conversations/${conversationId}/messages/${messageId}/delivery`,
    {},
    token,
  )
}
