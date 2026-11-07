export interface UserSummary {
  id: string
  username: string
  nickname: string
}

export interface LoginResponse {
  token: string
  user: UserSummary
}

export interface Conversation {
  id: string
  conversation_type: number
  name: string
  avatar_url: string
  last_seq: number
  last_read_seq: number
  unread_count: number
}

export interface Message {
  message_id: string
  conversation_id: string
  sender_id: string
  seq: number
  message_type: number
  content: string
  reply_to_message_id?: string
  created_at: string
}

export interface MessagePage {
  messages: Message[]
  has_more: boolean
}

export interface SendMessageResponse {
  message_id: string
  conversation_id: string
  seq: number
  created_at: string
  duplicate: boolean
}

export interface WsTicketResponse {
  ticket: string
  expires_in_seconds: number
  device_id: string
  device_name: string
  platform: number
}

export interface WsMessage {
  action: string
  message_id: string
  conversation_id?: string
  sender_id?: string
  seq?: number
  msg_type?: number
  content?: string
  url?: string
}

export interface ReadState {
  user_id: string
  last_read_seq: number
}

export interface DeliverySummary {
  delivered_devices: number
  delivered_users: number
}

export interface ApiErrorBody {
  error?: string
}
