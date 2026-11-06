package messaging

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"ginChatProject/internal/platform/snowflake"
	"github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Repository 负责消息相关数据库事务。
type Repository struct {
	db *gorm.DB
}

// NewRepository 创建消息 Repository。
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

type sendTextInput struct {
	ConversationID   uint64
	SenderID         uint64
	ClientMessageID  string
	Content          string
	ReplyToMessageID *uint64
}

// sendText 在单个事务中保存文本消息和 Outbox 事件。
func (r *Repository) sendText(
	ctx context.Context,
	input sendTextInput,
) (*message, bool, error) {
	existing, err := r.findByClientMessageID(ctx, input.SenderID, input.ClientMessageID)
	if err == nil {
		if existing.ConversationID != input.ConversationID {
			return nil, false, ErrClientMessageConflict
		}
		return existing, true, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, fmt.Errorf("查询客户端消息: %w", err)
	}

	now := time.Now().UTC()
	msg := &message{
		ID:               uint64(snowflake.Generate()),
		ConversationID:   input.ConversationID,
		SenderID:         input.SenderID,
		ClientMessageID:  input.ClientMessageID,
		MessageType:      messageTypeText,
		Content:          input.Content,
		ReplyToMessageID: input.ReplyToMessageID,
		CreatedAt:        now,
	}

	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var conversationRecord conversation
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", input.ConversationID).
			First(&conversationRecord).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrConversationNotFound
			}
			return fmt.Errorf("锁定会话: %w", err)
		}
		if conversationRecord.Status != conversationStatusActive {
			return ErrConversationClosed
		}

		var member conversationMember
		if err := tx.Where(
			"conversation_id = ? AND user_id = ? AND left_at IS NULL",
			input.ConversationID,
			input.SenderID,
		).First(&member).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotMember
			}
			return fmt.Errorf("校验会话成员: %w", err)
		}

		if input.ReplyToMessageID != nil {
			var replied message
			if err := tx.Where(
				"id = ? AND conversation_id = ?",
				*input.ReplyToMessageID,
				input.ConversationID,
			).First(&replied).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return ErrInvalidReply
				}
				return fmt.Errorf("校验引用消息: %w", err)
			}
		}

		var duplicate message
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("sender_id = ? AND client_msg_id = ?", input.SenderID, input.ClientMessageID).
			First(&duplicate).Error
		if err == nil {
			if duplicate.ConversationID != input.ConversationID {
				return ErrClientMessageConflict
			}
			msg = &duplicate
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("检查重复消息: %w", err)
		}

		msg.Seq = conversationRecord.LastSeq + 1
		if err := tx.Create(msg).Error; err != nil {
			return fmt.Errorf("保存消息: %w", err)
		}

		if err := tx.Model(&conversation{}).
			Where("id = ?", input.ConversationID).
			Updates(map[string]any{
				"last_seq":        msg.Seq,
				"last_message_id": msg.ID,
			}).Error; err != nil {
			return fmt.Errorf("更新会话序号: %w", err)
		}

		payload, err := json.Marshal(messageCreatedPayload{
			MessageID:      strconv.FormatUint(msg.ID, 10),
			ConversationID: strconv.FormatUint(msg.ConversationID, 10),
			SenderID:       strconv.FormatUint(msg.SenderID, 10),
			Seq:            msg.Seq,
		})
		if err != nil {
			return fmt.Errorf("序列化消息事件: %w", err)
		}

		event := &outboxEvent{
			EventID:       uint64(snowflake.Generate()),
			AggregateType: "conversation",
			AggregateID:   input.ConversationID,
			EventType:     "message.created.v1",
			Payload:       string(payload),
			Status:        outboxStatusPending,
			AvailableAt:   now,
			CreatedAt:     now,
		}
		if err := tx.Create(event).Error; err != nil {
			return fmt.Errorf("保存 Outbox 事件: %w", err)
		}
		return nil
	})
	if err == nil {
		return msg, false, nil
	}

	if isDuplicateKey(err) {
		existing, findErr := r.findByClientMessageID(ctx, input.SenderID, input.ClientMessageID)
		if findErr != nil {
			return nil, false, fmt.Errorf("读取重复消息: %w", findErr)
		}
		if existing.ConversationID != input.ConversationID {
			return nil, false, ErrClientMessageConflict
		}
		return existing, true, nil
	}

	return nil, false, err
}

func (r *Repository) findByClientMessageID(
	ctx context.Context,
	senderID uint64,
	clientMessageID string,
) (*message, error) {
	var msg message
	err := r.db.WithContext(ctx).
		Where("sender_id = ? AND client_msg_id = ?", senderID, clientMessageID).
		First(&msg).Error
	if err != nil {
		return nil, err
	}
	return &msg, nil
}

func isDuplicateKey(err error) bool {
	var mysqlErr *mysql.MySQLError
	return errors.As(err, &mysqlErr) && mysqlErr.Number == 1062
}
