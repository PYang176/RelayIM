package dto

import "ginChatProject/internal/model"

type SendMessageReq struct {
	// 接收者ID
	ReceiverID uint64 `json:"receiver_id" binding:"required"`
	// 发送类型
	SendType model.SendType `json:"send_type" binding:"required"`
	// 消息类型
	MsgType model.MsgType `json:"msg_type" binding:"required"`
	// 消息内容
	Content string `json:"content"`
	URL     string `json:"url"`
}
