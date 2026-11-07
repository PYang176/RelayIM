package messaging

import (
	"context"
	"errors"
	"ginChatProject/internal/middleware"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const maxMessageRequestBodyBytes = 1 << 20

// TextMessageSender 是 Handler 发送文本消息所需的最小业务能力。
type TextMessageSender interface {
	SendText(ctx context.Context, command SendTextCommand) (SendTextResult, error)
}

// MessageReader 是 Handler 查询历史消息所需的最小业务能力。
type MessageReader interface {
	ListMessages(ctx context.Context, command ListMessagesCommand) (MessagePage, error)
}

// Handler 处理新版消息 HTTP 请求。
type Handler struct {
	service TextMessageSender
	reader  MessageReader
}

// NewHandler 创建消息 Handler。
func NewHandler(service TextMessageSender, reader MessageReader) *Handler {
	return &Handler{
		service: service,
		reader:  reader,
	}
}

type sendTextRequest struct {
	// ClientMessageID 是客户端生成的消息幂等键。
	ClientMessageID string `json:"client_message_id"`

	// Content 是文本正文。
	Content string `json:"content"`

	// ReplyToMessageID 是可选引用消息 Snowflake ID 字符串。
	ReplyToMessageID *string `json:"reply_to_message_id"`
}

type sendTextResponse struct {
	// MessageID 是消息 Snowflake ID 字符串。
	MessageID string `json:"message_id"`

	// ConversationID 是会话 Snowflake ID 字符串。
	ConversationID string `json:"conversation_id"`

	// Seq 是消息在会话内的严格递增序号。
	Seq uint64 `json:"seq"`

	// CreatedAt 是服务端首次接受消息的 UTC 时间。
	CreatedAt time.Time `json:"created_at"`

	// Duplicate 表示本次请求命中了客户端幂等键。
	Duplicate bool `json:"duplicate"`
}

type messageHistoryResponse struct {
	// MessageID 是消息 Snowflake ID 字符串。
	MessageID string `json:"message_id"`

	// ConversationID 是会话 Snowflake ID 字符串。
	ConversationID string `json:"conversation_id"`

	// SenderID 是发送者 Snowflake ID 字符串。
	SenderID string `json:"sender_id"`

	// Seq 是消息在会话内的递增序号。
	Seq uint64 `json:"seq"`

	// MessageType 表示消息类型。
	MessageType int `json:"message_type"`

	// Content 是文本正文。
	Content string `json:"content"`

	// ReplyToMessageID 是可选引用消息 Snowflake ID 字符串。
	ReplyToMessageID *string `json:"reply_to_message_id,omitempty"`

	// CreatedAt 是消息创建的 UTC 时间。
	CreatedAt time.Time `json:"created_at"`
}

// SendText 处理 POST /api/v1/conversations/:conversation_id/messages。
func (h *Handler) SendText(c *gin.Context) {
	// 发送者身份只来自认证中间件，客户端伪造 sender_id 不会进入业务命令。
	userID, ok := middleware.CurrentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "登录用户信息无效"})
		return
	}

	conversationID, err := strconv.ParseUint(
		c.Param("conversation_id"),
		10,
		64,
	)
	if err != nil || conversationID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "conversation_id 格式错误"})
		return
	}

	// 限制原始请求体，避免超大 JSON 在业务校验前占用过多内存。
	c.Request.Body = http.MaxBytesReader(
		c.Writer,
		c.Request.Body,
		maxMessageRequestBodyBytes,
	)

	var request sendTextRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "请求体过大"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数格式错误"})
		return
	}

	replyToMessageID, ok := parseReplyToMessageID(c, request.ReplyToMessageID)
	if !ok {
		return
	}

	result, err := h.service.SendText(c.Request.Context(), SendTextCommand{
		ConversationID:   conversationID,
		SenderID:         userID,
		ClientMessageID:  request.ClientMessageID,
		Content:          request.Content,
		ReplyToMessageID: replyToMessageID,
	})
	if err != nil {
		writeMessagingError(c, err)
		return
	}

	status := http.StatusCreated
	if result.Duplicate {
		status = http.StatusOK
	}
	c.JSON(status, sendTextResponse{
		MessageID:      strconv.FormatUint(result.MessageID, 10),
		ConversationID: strconv.FormatUint(result.ConversationID, 10),
		Seq:            result.Seq,
		CreatedAt:      result.CreatedAt,
		Duplicate:      result.Duplicate,
	})
}

// ListMessages 处理 GET /api/v1/conversations/:conversation_id/messages。
func (h *Handler) ListMessages(c *gin.Context) {
	userID, ok := middleware.CurrentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "登录用户信息无效"})
		return
	}

	conversationID, err := strconv.ParseUint(
		c.Param("conversation_id"),
		10,
		64,
	)
	if err != nil || conversationID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "conversation_id 格式错误"})
		return
	}

	afterSeq, ok := parseOptionalUint64Query(c, "after_seq")
	if !ok {
		return
	}
	beforeSeq, ok := parseOptionalUint64Query(c, "before_seq")
	if !ok {
		return
	}
	limit, ok := parseOptionalIntQuery(c, "limit")
	if !ok {
		return
	}

	page, err := h.reader.ListMessages(c.Request.Context(), ListMessagesCommand{
		ConversationID: conversationID,
		UserID:         userID,
		AfterSeq:       afterSeq,
		BeforeSeq:      beforeSeq,
		Limit:          limit,
	})
	if err != nil {
		writeMessagingError(c, err)
		return
	}

	response := make([]messageHistoryResponse, 0, len(page.Messages))
	for _, message := range page.Messages {
		var replyTo *string
		if message.ReplyToMessageID != nil {
			value := strconv.FormatUint(*message.ReplyToMessageID, 10)
			replyTo = &value
		}
		response = append(response, messageHistoryResponse{
			MessageID:        strconv.FormatUint(message.ID, 10),
			ConversationID:   strconv.FormatUint(message.ConversationID, 10),
			SenderID:         strconv.FormatUint(message.SenderID, 10),
			Seq:              message.Seq,
			MessageType:      message.MessageType,
			Content:          message.Content,
			ReplyToMessageID: replyTo,
			CreatedAt:        message.CreatedAt,
		})
	}
	c.JSON(http.StatusOK, gin.H{
		"messages": response,
		"has_more": page.HasMore,
	})
}

func parseReplyToMessageID(
	c *gin.Context,
	raw *string,
) (*uint64, bool) {
	if raw == nil {
		return nil, true
	}

	value := strings.TrimSpace(*raw)
	replyToMessageID, err := strconv.ParseUint(value, 10, 64)
	if err != nil || replyToMessageID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "reply_to_message_id 格式错误"})
		return nil, false
	}
	return &replyToMessageID, true
}

func parseOptionalUint64Query(c *gin.Context, key string) (uint64, bool) {
	value := strings.TrimSpace(c.Query(key))
	if value == "" {
		return 0, true
	}
	parsed, err := strconv.ParseUint(value, 10, 64)
	if err != nil || parsed == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": key + " 格式错误"})
		return 0, false
	}
	return parsed, true
}

func parseOptionalIntQuery(c *gin.Context, key string) (int, bool) {
	value := strings.TrimSpace(c.Query(key))
	if value == "" {
		return 0, true
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": key + " 格式错误"})
		return 0, false
	}
	return parsed, true
}

func writeMessagingError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrInvalidMessage), errors.Is(err, ErrInvalidReply):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, ErrNotMember):
		c.JSON(http.StatusForbidden, gin.H{"error": ErrNotMember.Error()})
	case errors.Is(err, ErrConversationNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": ErrConversationNotFound.Error()})
	case errors.Is(err, ErrConversationClosed):
		c.JSON(http.StatusConflict, gin.H{"error": ErrConversationClosed.Error()})
	case errors.Is(err, ErrClientMessageConflict):
		c.JSON(http.StatusConflict, gin.H{"error": ErrClientMessageConflict.Error()})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "服务器内部错误"})
	}
}
