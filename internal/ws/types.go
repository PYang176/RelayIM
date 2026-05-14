package ws

// WS 信令类型（不写入数据库）
const (
	ActionMessage = "message" // 普通消息
	ActionAck     = "ack"     // 送达回执
	ActionRead    = "read"    // 已读回执
)

// WsMessage WebSocket 消息结构
type WsMessage struct {
	MessageID  string `json:"message_id"`
	SenderID   uint64 `json:"sender_id"`
	ReceiverID uint64 `json:"receiver_id"` //发送类型 1私聊 2群聊 3广播
	MsgType    int    `json:"msg_type"`    //消息类型 1文字 2图片 3音频 4文件
	Content    string `json:"content"`
	URL        string `json:"url"`
	Action     string `json:"action"`
}
