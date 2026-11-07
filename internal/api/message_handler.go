package api

import (
	"encoding/json"
	"fmt"
	"ginChatProject/internal/dto"
	"ginChatProject/internal/model"
	"ginChatProject/internal/service"
	"ginChatProject/internal/ws"
	"github.com/gin-gonic/gin"
	"net/http"
	"strconv"
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 构造websocket消息
	msg := ws.WsMessage{
		MessageID:  messageID,
		SenderID:   strconv.FormatUint(uint64(sendID), 10),
		ReceiverID: strconv.FormatUint(req.ReceiverID, 10),
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

// SendGroupMessage 发送群消息
func (h *MessageHandler) SendGroupMessage(c *gin.Context) {
	userID := c.MustGet("user_id").(uint)
	groupIDStr := c.Param("id")
	groupID, err := strconv.ParseUint(groupIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}

	var req struct {
		MsgType model.MsgType `json:"msg_type" binding:"required"`
		Content string        `json:"content"`
		URL     string        `json:"url"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 构造参数结构体
	params := service.SendGroupMessageParams{
		GroupID:  groupID,
		SenderID: uint64(userID),
		MsgType:  req.MsgType,
		Content:  req.Content,
		URL:      req.URL,
	}

	msg, err := h.msgService.SendGroupMessage(
		c.Request.Context(),
		params,
	)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 不再直接推送，交给 Redis Stream 消费者处理
	c.JSON(http.StatusOK, gin.H{
		"message":    "发送成功",
		"message_id": msg.MessageID,
		"note":       "消息已进入异步处理队列",
	})
}

// broadcastToGroup 推送群消息给在线成员
func (h *MessageHandler) broadcastToGroup(msg *model.Message, userIDs []uint64) {
	// 1. 准备推送消息
	pushMsg := ws.WsMessage{
		Action:     ws.ActionMessage,
		MessageID:  msg.MessageID,
		SenderID:   strconv.FormatUint(msg.SenderID, 10),
		ReceiverID: strconv.FormatUint(msg.ReceiverID, 10),
		MsgType:    int(msg.MsgType),
		Content:    msg.Content,
		URL:        msg.URL,
	}

	// 2. 序列化为 JSON
	data, err := json.Marshal(pushMsg)
	if err != nil {
		fmt.Printf("群消息序列化失败: %v\n", err)
		return
	}

	// 3. 遍历推送
	for _, userID := range userIDs {
		// 跳过发送者
		if userID == msg.SenderID {
			continue
		}

		// 只推送给在线用户
		if !ws.Manager.IsOnline(uint(userID)) {
			continue
		}

		// 发送消息
		ok := ws.Manager.Send(uint(userID), data)
		if !ok {
			fmt.Printf("用户 %d 推送失败\n", userID)
		}
	}
}
