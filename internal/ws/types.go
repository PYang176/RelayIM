package ws

// WS 信令类型（不写入数据库）
const (
	ActionMessage = "message" // 普通消息
	ActionAck     = "ack"     // 送达回执
	ActionRead    = "read"    // 已读回执

	// ActionMessageRecalled 表示一条已发送消息被撤回。
	ActionMessageRecalled = "message_recalled"
)

// WsMessage WebSocket 消息结构
type WsMessage struct {
	// MessageID 是消息 Snowflake ID 字符串。
	MessageID string `json:"message_id"`

	// ConversationID 是新版会话 Snowflake ID 字符串。
	ConversationID string `json:"conversation_id,omitempty"`

	// SenderID 是发送者 Snowflake 用户 ID 字符串，避免 JavaScript 精度丢失。
	SenderID string `json:"sender_id"`

	// RecalledBy 是撤回操作人 Snowflake 用户 ID 字符串。
	RecalledBy string `json:"recalled_by,omitempty"`

	// ReceiverID 是旧版接收者或群 ID 字符串，新版消息可留空。
	ReceiverID string `json:"receiver_id,omitempty"`

	// Seq 是消息在会话内的递增序号，用于客户端排序和去重。
	Seq uint64 `json:"seq,omitempty"`

	// MsgType 表示消息类型，1 文本、2 图片、3 音频、4 文件。
	MsgType int `json:"msg_type"`

	// Content 是文本消息正文。
	Content string `json:"content"`

	// URL 是旧版媒体地址字段。
	URL string `json:"url,omitempty"`

	// FileName 是附件消息的原始文件名。
	FileName string `json:"file_name,omitempty"`

	// Action 是 WebSocket 信令类型。
	Action string `json:"action"`

	// RecalledAt 是撤回发生的 UTC 时间。
	RecalledAt string `json:"recalled_at,omitempty"`
}
