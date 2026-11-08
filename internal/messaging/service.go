package messaging

import (
	"context"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	maxTextContentRunes = 8000
	defaultHistoryLimit = 20
	maxHistoryLimit     = 100
	senderRecallWindow  = 2 * time.Minute
	managerRecallWindow = 24 * time.Hour
)

// Service 提供消息业务用例。
type Service struct {
	repo    *Repository
	storage AttachmentStorage
}

// NewService 创建消息服务。
func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

// SendTextCommand 是发送文本消息所需的参数。
type SendTextCommand struct {
	ConversationID   uint64
	SenderID         uint64
	ClientMessageID  string
	Content          string
	ReplyToMessageID *uint64
}

// SendTextResult 是消息成功持久化后的结果。
type SendTextResult struct {
	MessageID      uint64
	ConversationID uint64
	Seq            uint64
	CreatedAt      time.Time
	Duplicate      bool
}

// SendAttachmentCommand 是发送图片或文件消息的参数。
type SendAttachmentCommand struct {
	ConversationID  uint64
	SenderID        uint64
	ClientMessageID string
	MessageType     int
	AttachmentID    uint64
	Content         string
}

// RecallMessageCommand 是撤回一条会话消息的参数。
type RecallMessageCommand struct {
	// ConversationID 是消息所属会话 Snowflake ID。
	ConversationID uint64

	// UserID 是执行撤回的当前登录用户 Snowflake ID。
	UserID uint64

	// MessageID 是待撤回消息 Snowflake ID。
	MessageID uint64
}

// ListMessagesCommand 是历史消息分页参数。
type ListMessagesCommand struct {
	// ConversationID 是待查询会话 Snowflake ID。
	ConversationID uint64

	// UserID 是当前登录用户 Snowflake ID。
	UserID uint64

	// AfterSeq 大于零时按升序返回该序号之后的消息，用于断线补拉。
	AfterSeq uint64

	// BeforeSeq 大于零时返回该序号之前的一页消息。
	BeforeSeq uint64

	// Limit 是分页数量，默认 20，最大 100。
	Limit int
}

// MessageResult 是接口层可安全使用的消息历史。
type MessageResult struct {
	// ID 是消息 Snowflake ID。
	ID uint64

	// ConversationID 是消息所属会话 Snowflake ID。
	ConversationID uint64

	// SenderID 是发送者 Snowflake 用户 ID。
	SenderID uint64

	// Seq 是消息在会话内的递增序号。
	Seq uint64

	// MessageType 表示消息类型。
	MessageType int

	// Content 是文本正文。
	Content string

	// ReplyToMessageID 是可选引用消息 Snowflake ID。
	ReplyToMessageID *uint64

	// AttachmentID 是可选附件 Snowflake ID。
	AttachmentID *uint64

	// AttachmentName 是可选附件原始文件名。
	AttachmentName string

	// AttachmentURL 是短时附件下载地址。
	AttachmentURL string

	// Recalled 表示消息已被撤回，内容和附件不再返回。
	Recalled bool

	// RecalledAt 是撤回发生的 UTC 时间。
	RecalledAt *time.Time

	// RecalledBy 是执行撤回的用户 Snowflake ID。
	RecalledBy *uint64

	// CreatedAt 是消息创建的 UTC 时间。
	CreatedAt time.Time
}

// MessagePage 是一页消息历史。
type MessagePage struct {
	// Messages 始终按 seq 升序返回。
	Messages []MessageResult

	// HasMore 表示当前查询方向仍存在更多消息。
	HasMore bool
}

// ReadStateResult 是会话成员当前已读位置。
type ReadStateResult struct {
	// UserID 是成员 Snowflake 用户 ID。
	UserID uint64

	// LastReadSeq 是成员最后已读序号。
	LastReadSeq uint64
}

// DeliverySummary 是一条消息的送达汇总。
type DeliverySummary struct {
	// DeliveredDevices 是已确认收到消息的设备数量。
	DeliveredDevices uint64

	// DeliveredUsers 是已确认收到消息的去重用户数量。
	DeliveredUsers uint64
}

// SendText 校验文本消息并调用数据库事务。
func (s *Service) SendText(
	ctx context.Context,
	command SendTextCommand,
) (SendTextResult, error) {
	// Service 是业务输入边界：先标准化字符串，避免由首尾空格造成的空消息、
	// 重复客户端 ID 或不可见输入进入数据库层。
	command.ClientMessageID = strings.TrimSpace(command.ClientMessageID)
	command.Content = strings.TrimSpace(command.Content)

	// ID、消息长度和引用参数在进入事务前完成校验。无效输入不会占用
	// 数据库连接，也不会触发热点会话行锁。
	if command.ConversationID == 0 || command.SenderID == 0 {
		return SendTextResult{}, ErrInvalidMessage
	}
	if command.ClientMessageID == "" || len(command.ClientMessageID) > 64 {
		return SendTextResult{}, ErrInvalidMessage
	}
	if command.Content == "" || utf8.RuneCountInString(command.Content) > maxTextContentRunes {
		return SendTextResult{}, ErrInvalidMessage
	}
	if command.ReplyToMessageID != nil && *command.ReplyToMessageID == 0 {
		return SendTextResult{}, ErrInvalidMessage
	}

	// duplicate=true 表示命中幂等重试。Service 对首次发送和重试返回同一种
	// 业务结果，同时把这个标记留给上层记录日志和指标。
	msg, duplicate, err := s.repo.sendMessage(ctx, sendMessageInput{
		ConversationID:   command.ConversationID,
		SenderID:         command.SenderID,
		ClientMessageID:  command.ClientMessageID,
		MessageType:      messageTypeText,
		Content:          command.Content,
		ReplyToMessageID: command.ReplyToMessageID,
	})
	if err != nil {
		return SendTextResult{}, err
	}

	// 只返回业务结果，不把包含 GORM 字段的内部消息模型暴露给 Handler。
	return SendTextResult{
		MessageID:      msg.ID,
		ConversationID: msg.ConversationID,
		Seq:            msg.Seq,
		CreatedAt:      msg.CreatedAt,
		Duplicate:      duplicate,
	}, nil
}

// SendAttachment 校验附件消息并调用数据库事务。
func (s *Service) SendAttachment(
	ctx context.Context,
	command SendAttachmentCommand,
) (SendTextResult, error) {
	command.ClientMessageID = strings.TrimSpace(command.ClientMessageID)
	command.Content = strings.TrimSpace(command.Content)
	if command.ConversationID == 0 ||
		command.SenderID == 0 ||
		command.AttachmentID == 0 ||
		(command.MessageType != messageTypeImage &&
			command.MessageType != messageTypeFile) {
		return SendTextResult{}, ErrInvalidMessage
	}
	if command.ClientMessageID == "" ||
		len(command.ClientMessageID) > 64 ||
		utf8.RuneCountInString(command.Content) > maxTextContentRunes {
		return SendTextResult{}, ErrInvalidMessage
	}

	msg, duplicate, err := s.repo.sendMessage(ctx, sendMessageInput{
		ConversationID:  command.ConversationID,
		SenderID:        command.SenderID,
		ClientMessageID: command.ClientMessageID,
		MessageType:     command.MessageType,
		Content:         command.Content,
		AttachmentID:    &command.AttachmentID,
	})
	if err != nil {
		return SendTextResult{}, err
	}
	return SendTextResult{
		MessageID:      msg.ID,
		ConversationID: msg.ConversationID,
		Seq:            msg.Seq,
		CreatedAt:      msg.CreatedAt,
		Duplicate:      duplicate,
	}, nil
}

// ListMessages 返回当前会话成员可见的一页历史消息。
func (s *Service) ListMessages(
	ctx context.Context,
	command ListMessagesCommand,
) (MessagePage, error) {
	if command.ConversationID == 0 || command.UserID == 0 {
		return MessagePage{}, ErrInvalidMessage
	}
	if command.AfterSeq > 0 && command.BeforeSeq > 0 {
		return MessagePage{}, ErrInvalidMessage
	}
	if command.Limit == 0 {
		command.Limit = defaultHistoryLimit
	}
	if command.Limit < 0 || command.Limit > maxHistoryLimit {
		return MessagePage{}, ErrInvalidMessage
	}

	records, hasMore, err := s.repo.listMessages(ctx, listMessagesInput{
		ConversationID: command.ConversationID,
		UserID:         command.UserID,
		AfterSeq:       command.AfterSeq,
		BeforeSeq:      command.BeforeSeq,
		Limit:          command.Limit,
	})
	if err != nil {
		return MessagePage{}, err
	}

	results := make([]MessageResult, 0, len(records))
	for _, record := range records {
		if record.RecalledAt != nil {
			results = append(results, MessageResult{
				ID:             record.ID,
				ConversationID: record.ConversationID,
				SenderID:       record.SenderID,
				Seq:            record.Seq,
				MessageType:    record.MessageType,
				Recalled:       true,
				RecalledAt:     record.RecalledAt,
				RecalledBy:     record.RecalledBy,
				CreatedAt:      record.CreatedAt,
			})
			continue
		}

		attachmentURL := ""
		if record.AttachmentObjectKey != nil && s.storage != nil {
			attachmentURL, _ = s.storage.PresignedGetURL(
				ctx,
				*record.AttachmentObjectKey,
				15*time.Minute,
			)
		}
		results = append(results, MessageResult{
			ID:               record.ID,
			ConversationID:   record.ConversationID,
			SenderID:         record.SenderID,
			Seq:              record.Seq,
			MessageType:      record.MessageType,
			Content:          record.Content,
			ReplyToMessageID: record.ReplyToMessageID,
			AttachmentID:     record.AttachmentID,
			AttachmentName:   stringValue(record.AttachmentName),
			AttachmentURL:    attachmentURL,
			RecalledAt:       record.RecalledAt,
			RecalledBy:       record.RecalledBy,
			CreatedAt:        record.CreatedAt,
		})
	}
	return MessagePage{
		Messages: results,
		HasMore:  hasMore,
	}, nil
}

// RecallMessage 校验撤回时间窗和群权限后写入撤回状态与可靠事件。
func (s *Service) RecallMessage(
	ctx context.Context,
	command RecallMessageCommand,
) error {
	if command.ConversationID == 0 ||
		command.UserID == 0 ||
		command.MessageID == 0 {
		return ErrInvalidMessage
	}
	return s.repo.recallMessage(
		ctx,
		command.ConversationID,
		command.UserID,
		command.MessageID,
		time.Now().UTC(),
	)
}

// AcknowledgeMessage 记录指定设备已收到消息。
func (s *Service) AcknowledgeMessage(
	ctx context.Context,
	userID uint64,
	deviceID uint64,
	messageID uint64,
) error {
	if userID == 0 || deviceID == 0 || messageID == 0 {
		return ErrInvalidMessage
	}
	return s.repo.acknowledgeMessage(ctx, userID, deviceID, messageID)
}

// MarkConversationRead 推进当前成员的会话已读游标。
func (s *Service) MarkConversationRead(
	ctx context.Context,
	userID uint64,
	conversationID uint64,
	seq uint64,
) error {
	if userID == 0 || conversationID == 0 || seq == 0 {
		return ErrInvalidMessage
	}
	return s.repo.markConversationRead(ctx, userID, conversationID, seq)
}

// ListReadStates 返回会话全部有效成员的已读游标。
func (s *Service) ListReadStates(
	ctx context.Context,
	userID uint64,
	conversationID uint64,
) ([]ReadStateResult, error) {
	if userID == 0 || conversationID == 0 {
		return nil, ErrInvalidMessage
	}
	return s.repo.listReadStates(ctx, userID, conversationID)
}

// GetDeliverySummary 返回指定消息的设备级送达汇总。
func (s *Service) GetDeliverySummary(
	ctx context.Context,
	userID uint64,
	conversationID uint64,
	messageID uint64,
) (DeliverySummary, error) {
	if userID == 0 || conversationID == 0 || messageID == 0 {
		return DeliverySummary{}, ErrInvalidMessage
	}
	return s.repo.getDeliverySummary(ctx, userID, conversationID, messageID)
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
