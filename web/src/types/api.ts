export interface UserSummary {
    id: string
    username: string
    nickname: string
    avatar_url?: string
    bio?: string
}

export interface LoginResponse {
    token: string
    refresh_token: string
    refresh_expires_at: string
    device_id: string
    user: UserSummary
}

export interface RefreshTokenResponse {
    token: string
    refresh_token: string
    refresh_expires_at: string
}

export interface Conversation {
    id: string
    conversation_type: number
    name: string
    title: string
    peer_user_id?: string
    avatar_url: string
    last_seq: number
    last_read_seq: number
    unread_count: number
    member_count: number
}

export interface ConversationMember {
    user_id: string
    username: string
    nickname: string
    avatar_url: string
    role: number
    joined_at: string
}

export interface Message {
    message_id: string
    conversation_id: string
    sender_id: string
    seq: number
    message_type: number
    content: string
    reply_to_message_id?: string
    attachment_id?: string
    attachment_url?: string
    attachment_name?: string
    recalled: boolean
    recalled_at?: string
    recalled_by?: string
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
    recalled_by?: string
    seq?: number
    msg_type?: number
    content?: string
    url?: string
    file_name?: string
    recalled_at?: string
}

export interface ReadState {
    user_id: string
    last_read_seq: number
}

export interface DeliverySummary {
    delivered_devices: number
    delivered_users: number
}

export interface AttachmentResponse {
    attachment_id: string
    message_type: number
    mime_type: string
    size_bytes: number
    filename: string
    url: string
}

export interface FriendUser {
    id: string
    username: string
    nickname: string
    avatar_url: string
}

export interface Friend {
    friendship_id: string
    user: FriendUser
    remark: string
}

export interface FriendRequest {
    id: string
    user: FriendUser
    status: number
    remark: string
    created_at: string
}

export interface ApiErrorBody {
    error?: string
}
