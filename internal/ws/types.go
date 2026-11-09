// Package ws 定义 WebSocket 协议和连接生命周期。
package ws

// WebSocket 信令类型，不写入数据库。
const (
	ActionMessage = "message" // 普通消息
	ActionAck     = "ack"     // 送达回执
	ActionRead    = "read"    // 已读回执

	// ActionMessageRecalled 表示一条已发送消息被撤回。
	ActionMessageRecalled = "message_recalled"

	// ActionGroupInvitation 表示收到新的群邀请。
	ActionGroupInvitation = "group_invitation"

	// ActionGroupInvitationCancelled 表示一条群邀请已被取消。
	ActionGroupInvitationCancelled = "group_invitation_cancelled"
)

// GroupInvitationMessage 是群邀请实时通知帧。
type GroupInvitationMessage struct {
	// Action 是群邀请或邀请取消信令。
	Action string `json:"action"`

	// InvitationID 是邀请 Snowflake ID 字符串。
	InvitationID string `json:"invitation_id"`

	// ConversationID 是目标群会话 Snowflake ID 字符串。
	ConversationID string `json:"conversation_id"`

	// ConversationName 是群名称。
	ConversationName string `json:"conversation_name,omitempty"`

	// InviterID 是邀请人 Snowflake ID 字符串。
	InviterID string `json:"inviter_id,omitempty"`

	// InviterName 是邀请人的公开昵称。
	InviterName string `json:"inviter_name,omitempty"`

	// InviteRemark 是邀请备注。
	InviteRemark string `json:"invite_remark,omitempty"`

	// ExpiresAt 是邀请失效的 UTC 时间。
	ExpiresAt string `json:"expires_at,omitempty"`
}

// WsMessage WebSocket 消息结构
type WsMessage struct {
	// MessageID 是消息 Snowflake ID 字符串。
	MessageID string `json:"message_id"`

	// ConversationID 是会话 Snowflake ID 字符串。
	ConversationID string `json:"conversation_id,omitempty"`

	// SenderID 是发送者 Snowflake 用户 ID 字符串，避免 JavaScript 精度丢失。
	SenderID string `json:"sender_id"`

	// RecalledBy 是撤回操作人 Snowflake 用户 ID 字符串。
	RecalledBy string `json:"recalled_by,omitempty"`

	// Seq 是消息在会话内的递增序号，用于客户端排序和去重。
	Seq uint64 `json:"seq,omitempty"`

	// MsgType 表示消息类型，1 文本、2 图片、3 文件、4 音频。
	MsgType int `json:"msg_type"`

	// Content 是文本消息正文。
	Content string `json:"content"`

	// URL 是附件内容访问路径。
	URL string `json:"url,omitempty"`

	// FileName 是附件消息的原始文件名。
	FileName string `json:"file_name,omitempty"`

	// Action 是 WebSocket 信令类型。
	Action string `json:"action"`

	// RecalledAt 是撤回发生的 UTC 时间。
	RecalledAt string `json:"recalled_at,omitempty"`
}
