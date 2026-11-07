package messaging

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"ginChatProject/internal/platform/rabbitmq"
	"gorm.io/gorm"
)

const messageCreatedConsumerName = "message-created-v1"

type permanentEventError struct {
	err error
}

func (e permanentEventError) Error() string {
	return e.err.Error()
}

func (e permanentEventError) Unwrap() error {
	return e.err
}

func (e permanentEventError) Permanent() bool {
	return true
}

// MessageCreatedProcessor 校验消息事件并记录消费幂等状态。
type MessageCreatedProcessor struct {
	db           *gorm.DB
	consumerName string
}

// NewMessageCreatedProcessor 创建消息创建事件处理器。
func NewMessageCreatedProcessor(db *gorm.DB) *MessageCreatedProcessor {
	return &MessageCreatedProcessor{
		db:           db,
		consumerName: messageCreatedConsumerName,
	}
}

// Process 校验事件引用的消息事实，并在同一事务中记录 Consumer 幂等结果。
func (p *MessageCreatedProcessor) Process(
	ctx context.Context,
	envelope rabbitmq.EventEnvelope,
) error {
	if envelope.EventType != rabbitmq.MessageCreatedRoutingKey {
		return permanentEventError{err: fmt.Errorf(
			"%w: 不支持的事件类型 %s",
			ErrInvalidEvent,
			envelope.EventType,
		)}
	}

	eventID, err := strconv.ParseUint(envelope.EventID, 10, 64)
	if err != nil || eventID == 0 {
		return permanentEventError{err: fmt.Errorf(
			"%w: event_id 格式错误",
			ErrInvalidEvent,
		)}
	}
	aggregateID, err := strconv.ParseUint(envelope.AggregateID, 10, 64)
	if err != nil || aggregateID == 0 {
		return permanentEventError{err: fmt.Errorf(
			"%w: aggregate_id 格式错误",
			ErrInvalidEvent,
		)}
	}

	var payload messageCreatedPayload
	if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
		return permanentEventError{err: fmt.Errorf(
			"%w: 解析消息载荷: %v",
			ErrInvalidEvent,
			err,
		)}
	}
	messageID, conversationID, senderID, err := parseMessageCreatedPayload(payload)
	if err != nil {
		return permanentEventError{err: err}
	}
	if conversationID != aggregateID {
		return permanentEventError{err: fmt.Errorf(
			"%w: aggregate_id 与会话 ID 不一致",
			ErrInvalidEvent,
		)}
	}

	err = p.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Exec(
			"INSERT IGNORE INTO processed_events (event_id, consumer_name, processed_at) "+
				"VALUES (?, ?, ?)",
			eventID,
			p.consumerName,
			time.Now().UTC(),
		)
		if result.Error != nil {
			return fmt.Errorf("记录 Consumer 幂等事件: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			return nil
		}

		var messageRecord message
		err := tx.Where(
			"id = ? AND conversation_id = ? AND sender_id = ? AND seq = ?",
			messageID,
			conversationID,
			senderID,
			payload.Seq,
		).First(&messageRecord).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return permanentEventError{err: fmt.Errorf(
				"%w: message_id=%d",
				ErrMessageEventNotFound,
				messageID,
			)}
		}
		if err != nil {
			return fmt.Errorf("校验消息事件事实: %w", err)
		}
		return nil
	})
	if err != nil {
		return err
	}
	return nil
}

func parseMessageCreatedPayload(
	payload messageCreatedPayload,
) (uint64, uint64, uint64, error) {
	messageID, err := strconv.ParseUint(payload.MessageID, 10, 64)
	if err != nil || messageID == 0 {
		return 0, 0, 0, fmt.Errorf("%w: message_id 格式错误", ErrInvalidEvent)
	}
	conversationID, err := strconv.ParseUint(payload.ConversationID, 10, 64)
	if err != nil || conversationID == 0 {
		return 0, 0, 0, fmt.Errorf("%w: conversation_id 格式错误", ErrInvalidEvent)
	}
	senderID, err := strconv.ParseUint(payload.SenderID, 10, 64)
	if err != nil || senderID == 0 {
		return 0, 0, 0, fmt.Errorf("%w: sender_id 格式错误", ErrInvalidEvent)
	}
	if payload.Seq == 0 {
		return 0, 0, 0, fmt.Errorf("%w: seq 必须大于 0", ErrInvalidEvent)
	}
	return messageID, conversationID, senderID, nil
}
