package messaging

import (
	"context"
	"errors"
	"ginChatProject/internal/middleware"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const maxMessageRequestBodyBytes = 1 << 20

// MessageSender 是 Handler 发送文本和附件消息所需的最小业务能力。
type MessageSender interface {
	SendText(ctx context.Context, command SendTextCommand) (SendTextResult, error)
	SendAttachment(
		ctx context.Context,
		command SendAttachmentCommand,
	) (SendTextResult, error)
	RecallMessage(ctx context.Context, command RecallMessageCommand) error
}

// MessageReader 是 Handler 查询历史消息所需的最小业务能力。
type MessageReader interface {
	ListMessages(ctx context.Context, command ListMessagesCommand) (MessagePage, error)
	SearchMessages(
		ctx context.Context,
		command SearchMessagesCommand,
	) (MessageSearchPage, error)
	ListReadStates(
		ctx context.Context,
		userID uint64,
		conversationID uint64,
	) ([]ReadStateResult, error)
	GetDeliverySummary(
		ctx context.Context,
		userID uint64,
		conversationID uint64,
		messageID uint64,
	) (DeliverySummary, error)
}

// AttachmentManager 提供附件上传和受控下载。
type AttachmentManager interface {
	UploadAttachment(
		ctx context.Context,
		command UploadAttachmentCommand,
	) (AttachmentResult, error)
	OpenAttachment(
		ctx context.Context,
		userID uint64,
		attachmentID uint64,
	) (OpenAttachmentResult, error)
	GetAttachmentInfo(
		ctx context.Context,
		userID uint64,
		attachmentID uint64,
	) (AttachmentResult, error)
}

// MetricsRecorder 接收消息和附件主链路的业务指标。
type MetricsRecorder interface {
	RecordMessageAccepted(messageType string, duplicate bool)
	RecordAttachmentUpload(success bool)
}

// Handler 处理消息 HTTP 请求。
type Handler struct {
	service     MessageSender
	reader      MessageReader
	attachments AttachmentManager
	metrics     MetricsRecorder
}

// NewHandler 创建消息 Handler。
func NewHandler(
	service MessageSender,
	reader MessageReader,
	attachments AttachmentManager,
) *Handler {
	return NewHandlerWithMetrics(service, reader, attachments, nil)
}

// NewHandlerWithMetrics 创建带业务指标的消息 Handler。
func NewHandlerWithMetrics(
	service MessageSender,
	reader MessageReader,
	attachments AttachmentManager,
	metrics MetricsRecorder,
) *Handler {
	return &Handler{
		service:     service,
		reader:      reader,
		attachments: attachments,
		metrics:     metrics,
	}
}

type sendTextRequest struct {
	// ClientMessageID 是客户端生成的消息幂等键。
	ClientMessageID string `json:"client_message_id"`

	// Content 是文本正文。
	Content string `json:"content"`

	// ReplyToMessageID 是可选引用消息 Snowflake ID 字符串。
	ReplyToMessageID *string `json:"reply_to_message_id"`

	// MessageType 表示消息类型，默认 1 文本、2 图片、3 文件。
	MessageType int `json:"message_type"`

	// AttachmentID 是图片或文件附件 Snowflake ID 字符串。
	AttachmentID *string `json:"attachment_id"`
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

	// AttachmentID 是可选附件 Snowflake ID 字符串。
	AttachmentID string `json:"attachment_id,omitempty"`

	// AttachmentURL 是受权限保护的附件下载地址。
	AttachmentURL string `json:"attachment_url,omitempty"`

	// AttachmentName 是附件原始文件名。
	AttachmentName string `json:"attachment_name,omitempty"`

	// Recalled 表示正文和附件已从普通历史响应中隐藏。
	Recalled bool `json:"recalled"`

	// RecalledAt 是撤回发生的 UTC 时间。
	RecalledAt *time.Time `json:"recalled_at,omitempty"`

	// RecalledBy 是执行撤回的用户 Snowflake ID 字符串。
	RecalledBy string `json:"recalled_by,omitempty"`

	// CreatedAt 是消息创建的 UTC 时间。
	CreatedAt time.Time `json:"created_at"`
}

type messageSearchResponse struct {
	// MessageID 是消息 Snowflake ID 字符串。
	MessageID string `json:"message_id"`

	// ConversationID 是消息所属会话 Snowflake ID 字符串。
	ConversationID string `json:"conversation_id"`

	// SenderID 是发送者 Snowflake ID 字符串。
	SenderID string `json:"sender_id"`

	// SenderName 是发送者昵称或用户名。
	SenderName string `json:"sender_name"`

	// Seq 是消息在会话内的递增序号。
	Seq uint64 `json:"seq"`

	// Content 是命中的消息正文。
	Content string `json:"content"`

	// CreatedAt 是消息创建的 UTC 时间。
	CreatedAt time.Time `json:"created_at"`
}

type readStateResponse struct {
	// UserID 是成员 Snowflake 用户 ID 字符串。
	UserID string `json:"user_id"`

	// LastReadSeq 是成员最后已读序号。
	LastReadSeq uint64 `json:"last_read_seq"`
}

// SendMessage 处理 POST /api/v1/conversations/:conversation_id/messages。
func (h *Handler) SendMessage(c *gin.Context) {
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

	messageType := request.MessageType
	if messageType == 0 {
		messageType = messageTypeText
	}

	var result SendTextResult
	if messageType == messageTypeText {
		result, err = h.service.SendText(c.Request.Context(), SendTextCommand{
			ConversationID:   conversationID,
			SenderID:         userID,
			ClientMessageID:  request.ClientMessageID,
			Content:          request.Content,
			ReplyToMessageID: replyToMessageID,
		})
	} else {
		attachmentID, ok := parseAttachmentID(c, request.AttachmentID)
		if !ok {
			return
		}
		result, err = h.service.SendAttachment(
			c.Request.Context(),
			SendAttachmentCommand{
				ConversationID:  conversationID,
				SenderID:        userID,
				ClientMessageID: request.ClientMessageID,
				MessageType:     messageType,
				AttachmentID:    attachmentID,
				Content:         request.Content,
			},
		)
	}
	if err != nil {
		writeMessagingError(c, err)
		return
	}
	if h.metrics != nil {
		h.metrics.RecordMessageAccepted(
			messageTypeMetricLabel(messageType),
			result.Duplicate,
		)
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
		attachmentID := ""
		attachmentURL := ""
		if message.AttachmentID != nil {
			attachmentID = strconv.FormatUint(*message.AttachmentID, 10)
			attachmentURL = message.AttachmentURL
		}
		recalledBy := ""
		if message.RecalledBy != nil {
			recalledBy = strconv.FormatUint(*message.RecalledBy, 10)
		}
		response = append(response, messageHistoryResponse{
			MessageID:        strconv.FormatUint(message.ID, 10),
			ConversationID:   strconv.FormatUint(message.ConversationID, 10),
			SenderID:         strconv.FormatUint(message.SenderID, 10),
			Seq:              message.Seq,
			MessageType:      message.MessageType,
			Content:          message.Content,
			ReplyToMessageID: replyTo,
			AttachmentID:     attachmentID,
			AttachmentURL:    attachmentURL,
			AttachmentName:   message.AttachmentName,
			Recalled:         message.Recalled,
			RecalledAt:       message.RecalledAt,
			RecalledBy:       recalledBy,
			CreatedAt:        message.CreatedAt,
		})
	}
	c.JSON(http.StatusOK, gin.H{
		"messages": response,
		"has_more": page.HasMore,
	})
}

// SearchMessages 处理 GET /api/v1/conversations/:conversation_id/messages/search。
func (h *Handler) SearchMessages(c *gin.Context) {
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
	beforeSeq, ok := parseOptionalUint64Query(c, "before_seq")
	if !ok {
		return
	}
	limit, ok := parseOptionalIntQuery(c, "limit")
	if !ok {
		return
	}

	page, err := h.reader.SearchMessages(
		c.Request.Context(),
		SearchMessagesCommand{
			ConversationID: conversationID,
			UserID:         userID,
			Query:          c.Query("q"),
			BeforeSeq:      beforeSeq,
			Limit:          limit,
		},
	)
	if err != nil {
		writeMessagingError(c, err)
		return
	}

	response := make([]messageSearchResponse, 0, len(page.Messages))
	for _, message := range page.Messages {
		response = append(response, messageSearchResponse{
			MessageID:      strconv.FormatUint(message.ID, 10),
			ConversationID: strconv.FormatUint(message.ConversationID, 10),
			SenderID:       strconv.FormatUint(message.SenderID, 10),
			SenderName:     message.SenderName,
			Seq:            message.Seq,
			Content:        message.Content,
			CreatedAt:      message.CreatedAt,
		})
	}
	c.JSON(http.StatusOK, gin.H{
		"messages":        response,
		"next_before_seq": page.NextBeforeSeq,
		"has_more":        page.HasMore,
	})
}

// RecallMessage 处理 DELETE /api/v1/conversations/:conversation_id/messages/:message_id。
func (h *Handler) RecallMessage(c *gin.Context) {
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
	messageID, err := strconv.ParseUint(c.Param("message_id"), 10, 64)
	if err != nil || messageID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "message_id 格式错误"})
		return
	}
	if err := h.service.RecallMessage(
		c.Request.Context(),
		RecallMessageCommand{
			ConversationID: conversationID,
			UserID:         userID,
			MessageID:      messageID,
		},
	); err != nil {
		writeMessagingError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// ListReadStates 处理 GET /api/v1/conversations/:conversation_id/read-states。
func (h *Handler) ListReadStates(c *gin.Context) {
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

	results, err := h.reader.ListReadStates(
		c.Request.Context(),
		userID,
		conversationID,
	)
	if err != nil {
		writeMessagingError(c, err)
		return
	}

	response := make([]readStateResponse, 0, len(results))
	for _, result := range results {
		response = append(response, readStateResponse{
			UserID:      strconv.FormatUint(result.UserID, 10),
			LastReadSeq: result.LastReadSeq,
		})
	}
	c.JSON(http.StatusOK, gin.H{"read_states": response})
}

// GetDeliverySummary 处理消息设备送达汇总查询。
func (h *Handler) GetDeliverySummary(c *gin.Context) {
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
	messageID, err := strconv.ParseUint(c.Param("message_id"), 10, 64)
	if err != nil || messageID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "message_id 格式错误"})
		return
	}

	summary, err := h.reader.GetDeliverySummary(
		c.Request.Context(),
		userID,
		conversationID,
		messageID,
	)
	if err != nil {
		writeMessagingError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"delivered_devices": summary.DeliveredDevices,
		"delivered_users":   summary.DeliveredUsers,
	})
}

// UploadAttachment 处理 POST /api/v1/attachments。
func (h *Handler) UploadAttachment(c *gin.Context) {
	userID, ok := middleware.CurrentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "登录用户信息无效"})
		return
	}
	uploadSucceeded := false
	defer func() {
		if h.metrics != nil {
			h.metrics.RecordAttachmentUpload(uploadSucceeded)
		}
	}()
	c.Request.Body = http.MaxBytesReader(
		c.Writer,
		c.Request.Body,
		maxAttachmentBytes+(1<<20),
	)
	fileHeader, err := c.FormFile("file")
	if err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			c.JSON(
				http.StatusRequestEntityTooLarge,
				gin.H{"error": "附件不能超过 25 MB"},
			)
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "请选择要上传的文件"})
		return
	}
	if fileHeader.Size <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "不能上传空文件"})
		return
	}
	if fileHeader.Size > maxAttachmentBytes {
		c.JSON(
			http.StatusRequestEntityTooLarge,
			gin.H{"error": "附件不能超过 25 MB"},
		)
		return
	}
	file, err := fileHeader.Open()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "读取上传文件失败"})
		return
	}
	defer file.Close()

	detectBuffer := make([]byte, 512)
	n, readErr := io.ReadFull(file, detectBuffer)
	if readErr != nil && !errors.Is(readErr, io.ErrUnexpectedEOF) && !errors.Is(readErr, io.EOF) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "读取上传文件失败"})
		return
	}
	contentType := http.DetectContentType(detectBuffer[:n])
	if contentType == "text/html" || contentType == "image/svg+xml" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "不支持该文件类型"})
		return
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "处理上传文件失败"})
		return
	}

	result, err := h.attachments.UploadAttachment(
		c.Request.Context(),
		UploadAttachmentCommand{
			OwnerID:     userID,
			Filename:    fileHeader.Filename,
			ContentType: contentType,
			Size:        fileHeader.Size,
			Reader:      file,
		},
	)
	if err != nil {
		writeMessagingError(c, err)
		return
	}
	uploadSucceeded = true
	c.JSON(http.StatusCreated, gin.H{
		"attachment_id": strconv.FormatUint(result.ID, 10),
		"message_type":  result.MessageType,
		"mime_type":     result.MIMEType,
		"size_bytes":    result.SizeBytes,
		"filename":      result.Filename,
		"url":           result.URL,
	})
}

// messageTypeMetricLabel 把消息类型转换为 Prometheus 指标标签
func messageTypeMetricLabel(messageType int) string {
	switch messageType {
	case messageTypeImage:
		return "image"
	case messageTypeFile:
		return "file"
	default:
		return "text"
	}
}

// DownloadAttachment 处理 GET /api/v1/attachments/:id/content。
func (h *Handler) DownloadAttachment(c *gin.Context) {
	userID, ok := middleware.CurrentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "登录用户信息无效"})
		return
	}
	attachmentID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || attachmentID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "附件 ID 格式错误"})
		return
	}
	result, err := h.attachments.OpenAttachment(
		c.Request.Context(),
		userID,
		attachmentID,
	)
	if err != nil {
		writeMessagingError(c, err)
		return
	}
	defer result.Reader.Close()

	disposition := "attachment"
	if len(result.MIMEType) >= 6 && result.MIMEType[:6] == "image/" {
		disposition = "inline"
	}
	contentDisposition := mime.FormatMediaType(
		disposition,
		map[string]string{"filename": result.Filename},
	)
	c.Header("Content-Disposition", contentDisposition)
	c.DataFromReader(
		http.StatusOK,
		result.SizeBytes,
		result.MIMEType,
		result.Reader,
		nil,
	)
}

// GetAttachmentInfo 处理 GET /api/v1/attachments/:id。
func (h *Handler) GetAttachmentInfo(c *gin.Context) {
	userID, ok := middleware.CurrentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "登录用户信息无效"})
		return
	}
	attachmentID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || attachmentID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "附件 ID 格式错误"})
		return
	}
	result, err := h.attachments.GetAttachmentInfo(
		c.Request.Context(),
		userID,
		attachmentID,
	)
	if err != nil {
		writeMessagingError(c, err)
		return
	}
	c.Header("Cache-Control", "private, max-age=300")
	c.JSON(http.StatusOK, gin.H{
		"attachment_id": strconv.FormatUint(result.ID, 10),
		"message_type":  result.MessageType,
		"mime_type":     result.MIMEType,
		"size_bytes":    result.SizeBytes,
		"filename":      result.Filename,
		"url":           result.URL,
	})
}

// parseReplyToMessageID 解析引用消息 ID并校验参数边界
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

// parseAttachmentID 解析附件 ID并校验参数边界
func parseAttachmentID(
	c *gin.Context,
	raw *string,
) (uint64, bool) {
	if raw == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "附件消息缺少 attachment_id"})
		return 0, false
	}
	attachmentID, err := strconv.ParseUint(strings.TrimSpace(*raw), 10, 64)
	if err != nil || attachmentID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "attachment_id 格式错误"})
		return 0, false
	}
	return attachmentID, true
}

// parseOptionalUint64Query 解析可选无符号整数查询参数并校验格式
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

// parseOptionalIntQuery 解析可选整数查询参数并校验格式
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

// writeMessagingError 把消息业务错误映射为 HTTP 状态和响应
func writeMessagingError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrInvalidMessage),
		errors.Is(err, ErrInvalidReply),
		errors.Is(err, ErrInvalidSearchQuery):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, ErrInvalidReadSequence):
		c.JSON(http.StatusBadRequest, gin.H{"error": ErrInvalidReadSequence.Error()})
	case errors.Is(err, ErrNotMember):
		c.JSON(http.StatusForbidden, gin.H{"error": ErrNotMember.Error()})
	case errors.Is(err, ErrUserBlocked):
		c.JSON(http.StatusForbidden, gin.H{"error": ErrUserBlocked.Error()})
	case errors.Is(err, ErrDeviceUnavailable):
		c.JSON(http.StatusForbidden, gin.H{"error": ErrDeviceUnavailable.Error()})
	case errors.Is(err, ErrInvalidAttachment):
		c.JSON(http.StatusBadRequest, gin.H{"error": ErrInvalidAttachment.Error()})
	case errors.Is(err, ErrAttachmentNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": ErrAttachmentNotFound.Error()})
	case errors.Is(err, ErrAttachmentAlreadyUsed):
		c.JSON(http.StatusConflict, gin.H{"error": ErrAttachmentAlreadyUsed.Error()})
	case errors.Is(err, ErrAttachmentStorageUnavailable):
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": ErrAttachmentStorageUnavailable.Error()})
	case errors.Is(err, ErrConversationNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": ErrConversationNotFound.Error()})
	case errors.Is(err, ErrMessageNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": ErrMessageNotFound.Error()})
	case errors.Is(err, ErrRecallForbidden):
		c.JSON(http.StatusForbidden, gin.H{"error": ErrRecallForbidden.Error()})
	case errors.Is(err, ErrRecallExpired):
		c.JSON(http.StatusConflict, gin.H{"error": ErrRecallExpired.Error()})
	case errors.Is(err, ErrConversationClosed):
		c.JSON(http.StatusConflict, gin.H{"error": ErrConversationClosed.Error()})
	case errors.Is(err, ErrClientMessageConflict):
		c.JSON(http.StatusConflict, gin.H{"error": ErrClientMessageConflict.Error()})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "服务器内部错误"})
	}
}
