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
    current_user_role: number
    pinned: boolean
    unread_count: number
    member_count: number
    direct_friendship_active: boolean
    updated_at: string
}

export interface ConversationPage {
    conversations: Conversation[]
    next_cursor: string
    has_more: boolean
}

export interface ConversationMember {
    user_id: string
    username: string
    nickname: string
    avatar_url: string
    role: number
    joined_at: string
}

export interface ReceivedGroupInvitation {
    invitation_id: string
    conversation_id: string
    conversation_name: string
    avatar_url: string
    inviter_id: string
    inviter_name: string
    invite_remark: string
    created_at: string
    expires_at: string
}

export interface GroupInvitationPage {
    invitations: ReceivedGroupInvitation[]
    next_cursor: string
    has_more: boolean
}

export interface GroupPendingInvitation {
    invitation_id: string
    invitee: FriendUser
    inviter_id: string
    invite_remark: string
    status: number
    response_remark: string
    created_at: string
    expires_at: string
    responded_at?: string
}

export interface GroupPendingInvitationPage {
    invitations: GroupPendingInvitation[]
    next_cursor: string
    has_more: boolean
}

export interface InvitationAcceptance {
    invitation_id: string
    conversation_id: string
    member_count: number
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

export interface MessageSearchResult {
    message_id: string
    conversation_id: string
    sender_id: string
    sender_name: string
    seq: number
    content: string
    created_at: string
}

export interface MessageSearchPage {
    messages: MessageSearchResult[]
    next_before_seq: number
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
    peer_user_id?: string
    relationship_active?: boolean
    friend_visible?: boolean
    recalled_by?: string
    seq?: number
    msg_type?: number
    content?: string
    url?: string
    file_name?: string
    recalled_at?: string
    invitation_id?: string
    conversation_name?: string
    inviter_name?: string
    invite_remark?: string
    expires_at?: string
    trace_id?: string
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
    blocked_by_me: boolean
    relationship_active: boolean
}

export interface FriendRelationshipStatus {
    target_user_id: string
    can_message: boolean
    can_request: boolean
    has_pending_request: boolean
    requested_by_me: boolean
    blocked_by_me: boolean
}

export interface BlockedUser {
    user: FriendUser
    created_at: string
}

export interface BlockedUserPage {
    blocked_users: BlockedUser[]
    next_cursor: string
    has_more: boolean
}

export interface FriendRequest {
    id: string
    user: FriendUser
    status: number
    remark: string
    created_at: string
}

export interface FriendPage {
    friends: Friend[]
    next_cursor: string
    has_more: boolean
}

export interface FriendRequestPage {
    requests: FriendRequest[]
    next_cursor: string
    has_more: boolean
}

export interface ApiErrorBody {
    error?: string
}
