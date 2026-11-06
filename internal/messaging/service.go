package messaging

import (
	"context"
	"strings"
	"time"
	"unicode/utf8"
)

const maxTextContentRunes = 8000

// Service 提供消息业务用例。
type Service struct {
	repo *Repository
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
	msg, duplicate, err := s.repo.sendText(ctx, sendTextInput{
		ConversationID:   command.ConversationID,
		SenderID:         command.SenderID,
		ClientMessageID:  command.ClientMessageID,
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
