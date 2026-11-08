package messaging

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"ginChatProject/internal/platform/rabbitmq"
	"ginChatProject/internal/ws"
	"gorm.io/gorm"
)

const messageEventsConsumerName = "message-events-v1"

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

// MessageEventProcessor 校验消息领域事件、记录幂等状态并推送到在线连接。
type MessageEventProcessor struct {
	db           *gorm.DB
	consumerName string
	delivery     EventDeliveryRouter
}

// EventDeliveryRouter 把事件按用户当前所在实例进行实时路由。
type EventDeliveryRouter interface {
	Route(ctx context.Context, userIDs []uint64, message []byte) error
}

// NewMessageEventProcessor 创建消息领域事件处理器。
func NewMessageEventProcessor(
	db *gorm.DB,
	delivery EventDeliveryRouter,
) *MessageEventProcessor {
	return &MessageEventProcessor{
		db:           db,
		consumerName: messageEventsConsumerName,
		delivery:     delivery,
	}
}

// Process 按事件类型分发消息创建和撤回处理。
func (p *MessageEventProcessor) Process(
	ctx context.Context,
	envelope rabbitmq.EventEnvelope,
) error {
	switch envelope.EventType {
	case rabbitmq.MessageCreatedRoutingKey:
		return p.processMessageCreated(ctx, envelope)
	case rabbitmq.MessageRecalledRoutingKey:
		return p.processMessageRecalled(ctx, envelope)
	default:
		return permanentEventError{err: fmt.Errorf(
			"%w: 不支持的事件类型 %s",
			ErrInvalidEvent,
			envelope.EventType,
		)}
	}
}

func (p *MessageEventProcessor) processMessageCreated(
	ctx context.Context,
	envelope rabbitmq.EventEnvelope,
) error {
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
	messageType := payload.MessageType
	if messageType == 0 {
		messageType = messageTypeText
	}
	if messageType != messageTypeText &&
		messageType != messageTypeImage &&
		messageType != messageTypeFile {
		return permanentEventError{err: fmt.Errorf(
			"%w: 不支持的消息类型 %d",
			ErrInvalidEvent,
			messageType,
		)}
	}
	var attachmentID uint64
	if messageType != messageTypeText {
		attachmentID, err = strconv.ParseUint(payload.AttachmentID, 10, 64)
		if err != nil || attachmentID == 0 {
			return permanentEventError{err: fmt.Errorf(
				"%w: attachment_id 格式错误",
				ErrInvalidEvent,
			)}
		}
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
		if messageRecord.MessageType != messageType {
			return permanentEventError{err: fmt.Errorf(
				"%w: 消息类型与数据库不一致",
				ErrInvalidEvent,
			)}
		}
		if messageType != messageTypeText &&
			(messageRecord.AttachmentID == nil ||
				*messageRecord.AttachmentID != attachmentID) {
			return permanentEventError{err: fmt.Errorf(
				"%w: 消息附件与数据库不一致",
				ErrInvalidEvent,
			)}
		}

		// 撤回事件可能先于滞后的创建事件被处理。撤回状态已经提交时，
		// 不再把旧正文和附件推送到在线客户端。
		if messageRecord.RecalledAt != nil {
			return nil
		}

		var recipientIDs []uint64
		if err := tx.Table("conversation_members").
			Where(
				"conversation_id = ? AND left_at IS NULL",
				conversationID,
			).
			Pluck("user_id", &recipientIDs).Error; err != nil {
			return fmt.Errorf("查询消息在线接收者: %w", err)
		}

		if p.delivery != nil && len(recipientIDs) > 0 {
			attachmentURL := ""
			if attachmentID > 0 {
				attachmentURL = fmt.Sprintf(
					"/api/v1/attachments/%d/content",
					attachmentID,
				)
			}
			realtimePayload, err := json.Marshal(ws.WsMessage{
				MessageID:      strconv.FormatUint(messageID, 10),
				ConversationID: strconv.FormatUint(conversationID, 10),
				SenderID:       strconv.FormatUint(senderID, 10),
				Seq:            payload.Seq,
				MsgType:        messageType,
				Content:        messageRecord.Content,
				URL:            attachmentURL,
				FileName:       payload.AttachmentName,
				Action:         ws.ActionMessage,
			})
			if err != nil {
				return fmt.Errorf("序列化在线消息: %w", err)
			}
			if err := p.delivery.Route(
				ctx,
				recipientIDs,
				realtimePayload,
			); err != nil {
				return fmt.Errorf("跨实例路由在线消息: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	return nil
}

func (p *MessageEventProcessor) processMessageRecalled(
	ctx context.Context,
	envelope rabbitmq.EventEnvelope,
) error {
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

	var payload messageRecalledPayload
	if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
		return permanentEventError{err: fmt.Errorf(
			"%w: 解析撤回载荷: %v",
			ErrInvalidEvent,
			err,
		)}
	}
	messageID, err := strconv.ParseUint(payload.MessageID, 10, 64)
	if err != nil || messageID == 0 {
		return permanentEventError{err: fmt.Errorf(
			"%w: message_id 格式错误",
			ErrInvalidEvent,
		)}
	}
	conversationID, err := strconv.ParseUint(payload.ConversationID, 10, 64)
	if err != nil || conversationID == 0 || conversationID != aggregateID {
		return permanentEventError{err: fmt.Errorf(
			"%w: conversation_id 格式错误或与聚合 ID 不一致",
			ErrInvalidEvent,
		)}
	}
	senderID, err := strconv.ParseUint(payload.SenderID, 10, 64)
	if err != nil || senderID == 0 {
		return permanentEventError{err: fmt.Errorf(
			"%w: sender_id 格式错误",
			ErrInvalidEvent,
		)}
	}
	recalledBy, err := strconv.ParseUint(payload.RecalledBy, 10, 64)
	if err != nil || recalledBy == 0 || payload.Seq == 0 {
		return permanentEventError{err: fmt.Errorf(
			"%w: 撤回人或消息序号错误",
			ErrInvalidEvent,
		)}
	}

	return p.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Exec(
			"INSERT IGNORE INTO processed_events (event_id, consumer_name, processed_at) "+
				"VALUES (?, ?, ?)",
			eventID,
			p.consumerName,
			time.Now().UTC(),
		)
		if result.Error != nil {
			return fmt.Errorf("记录撤回 Consumer 幂等事件: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			return nil
		}

		var messageRecord message
		err := tx.Where(
			"id = ? AND conversation_id = ? AND sender_id = ? AND seq = ? "+
				"AND recalled_by = ? AND recalled_at IS NOT NULL",
			messageID,
			conversationID,
			senderID,
			payload.Seq,
			recalledBy,
		).First(&messageRecord).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return permanentEventError{err: fmt.Errorf(
				"%w: message_id=%d",
				ErrMessageEventNotFound,
				messageID,
			)}
		}
		if err != nil {
			return fmt.Errorf("校验消息撤回事实: %w", err)
		}

		var recipientIDs []uint64
		if err := tx.Table("conversation_members").
			Where(
				"conversation_id = ? AND left_at IS NULL",
				conversationID,
			).
			Pluck("user_id", &recipientIDs).Error; err != nil {
			return fmt.Errorf("查询撤回事件在线接收者: %w", err)
		}
		if p.delivery == nil || len(recipientIDs) == 0 {
			return nil
		}

		realtimePayload, err := json.Marshal(ws.WsMessage{
			MessageID:      strconv.FormatUint(messageID, 10),
			ConversationID: strconv.FormatUint(conversationID, 10),
			SenderID:       strconv.FormatUint(senderID, 10),
			RecalledBy:     strconv.FormatUint(recalledBy, 10),
			Seq:            payload.Seq,
			Action:         ws.ActionMessageRecalled,
			RecalledAt:     payload.RecalledAt.UTC().Format(time.RFC3339Nano),
		})
		if err != nil {
			return fmt.Errorf("序列化在线撤回事件: %w", err)
		}
		if err := p.delivery.Route(
			ctx,
			recipientIDs,
			realtimePayload,
		); err != nil {
			return fmt.Errorf("跨实例路由撤回事件: %w", err)
		}
		return nil
	})
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
