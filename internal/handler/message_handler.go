package handler

import (
	"encoding/json"
	"fmt"
	"ginChatProject/internal/dto"
	"ginChatProject/internal/service"
	"ginChatProject/internal/ws"
	"github.com/gin-gonic/gin"
	"net/http"
)

type MessageHandler struct {
	msgService *service.MessageService
}

func NewMessageHandler(svc *service.MessageService) *MessageHandler {
	return &MessageHandler{msgService: svc}
}

// Send 发送消息
func (h *MessageHandler) Send(c *gin.Context) {
	// 从token中拿到发送人id
	sendID := c.MustGet("user_id").(uint)

	// 绑定请求参数
	var req dto.SendMessageReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 构造 Service 参数
	params := service.SendMessageParams{
		SenderID:   uint64(sendID),
		ReceiverID: req.ReceiverID,
		SendType:   req.SendType,
		MsgType:    req.MsgType,
		Content:    req.Content,
		URL:        req.URL,
	}
	// 调用 Service 发送消息
	messageID, err := h.msgService.Send(c.Request.Context(), params)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "消息发送失败"})
		return
	}

	// 构造websocket消息
	msg := ws.WsMessage{
		MessageID:  messageID,
		SenderID:   uint64(sendID),
		ReceiverID: req.ReceiverID,
		MsgType:    int(req.MsgType),
		Content:    req.Content,
		URL:        req.URL,
	}

	// 序列化消息并发送
	data, _ := json.Marshal(msg)
	ok := ws.Manager.Send(uint(req.ReceiverID), data)
	if !ok {
		c.JSON(http.StatusOK, gin.H{
			"message":    "用户不在线，消息已暂存",
			"message_id": messageID,
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"message":    "消息发送成功",
		"message_id": messageID,
	})
}

// Count 查看在线人数
func (h *MessageHandler) Count(c *gin.Context) {
	str := fmt.Sprintf("在线人数为%d", ws.Manager.Count())
	c.JSON(http.StatusOK, str)
}

// PullOffline 离线消息推送
func (h *MessageHandler) PullOffline(c *gin.Context) {
	// 从token中拿到接收者id
	userID := c.MustGet("user_id").(uint)

	// 获取未推送的离线消息
	msgs, err := h.msgService.PullOfflineMessages(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "拉取离线消息失败"})
		return
	}

	// 返回离线消息
	c.JSON(http.StatusOK, gin.H{
		"offline_messages": msgs,
	})
}
