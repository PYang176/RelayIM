package messaging

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
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

type listMessagesInput struct {
	ConversationID uint64
	UserID         uint64
	AfterSeq       uint64
	BeforeSeq      uint64
	Limit          int
}

// sendText 在单个事务中保存文本消息和 Outbox 事件。
func (r *Repository) sendText(
	ctx context.Context,
	input sendTextInput,
) (*message, bool, error) {
	// 第一层幂等快速路径：客户端超时重试通常能直接拿到原消息，
	// 不进入会话行锁，减少重复请求对热点会话的争用。
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

	// Snowflake ID、事件 ID 和 UTC 时间在事务前生成。
	// 事务失败时这些 ID 会被废弃，不会在数据库中留下半条消息。
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
		// 事务第一条业务查询就锁住会话行。同一会话的发送请求串行分配
		// seq，不同会话仍可并发发送。
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

		// 外键只能证明用户存在，不能证明用户仍是会话成员。
		// left_at 为空表示当前有效成员，退群后不能继续发送。
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

		// 引用消息必须属于当前会话。数据库自引用外键只能保证被引用消息
		// 存在，无法表达“两条消息属于同一会话”这个跨行规则。
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

		// 拿到会话锁后再检查一次幂等键。并发重试可能同时穿过事务外的
		// 快速路径，这里是同一会话内的最终判重点。
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

		// 当前事务持有会话行锁，LastSeq 在读取和更新之间不会被其他
		// 发送者修改，因此当前会话的 seq 能保持严格连续。
		msg.Seq = conversationRecord.LastSeq + 1
		if err := tx.Create(msg).Error; err != nil {
			return fmt.Errorf("保存消息: %w", err)
		}

		// 会话摘要和消息在同一事务提交。任一更新失败都会回滚消息，
		// 避免 last_seq 与消息表不一致。
		if err := tx.Model(&conversation{}).
			Where("id = ?", input.ConversationID).
			Updates(map[string]any{
				"last_seq":        msg.Seq,
				"last_message_id": msg.ID,
			}).Error; err != nil {
			return fmt.Errorf("更新会话序号: %w", err)
		}

		// 事件载荷只保留 Consumer 定位消息所需的最小字段，并把 Snowflake
		// ID 转成字符串，避免前端或跨语言消费者出现数字精度丢失。
		payload, err := json.Marshal(messageCreatedPayload{
			MessageID:      strconv.FormatUint(msg.ID, 10),
			ConversationID: strconv.FormatUint(msg.ConversationID, 10),
			SenderID:       strconv.FormatUint(msg.SenderID, 10),
			Seq:            msg.Seq,
		})
		if err != nil {
			return fmt.Errorf("序列化消息事件: %w", err)
		}

		// Outbox 与消息共用本次事务。事务提交表示业务数据和待发布事件
		// 同时成功，RabbitMQ 发布器之后才读取该事件。
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

	// 不同会话可能并发使用同一个客户端消息 ID，唯一索引会返回 1062。
	// 此时事务已回滚，需要按唯一键读取先提交的原消息并返回同一结果。
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
	// 唯一索引 (sender_id, client_msg_id) 为这条查询提供精确查找路径。
	var msg message
	err := r.db.WithContext(ctx).
		Where("sender_id = ? AND client_msg_id = ?", senderID, clientMessageID).
		First(&msg).Error
	if err != nil {
		return nil, err
	}
	return &msg, nil
}

// listMessages 校验会话成员权限，并按请求游标返回一页历史消息。
func (r *Repository) listMessages(
	ctx context.Context,
	input listMessagesInput,
) ([]message, bool, error) {
	var conversationRecord conversation
	err := r.db.WithContext(ctx).
		Where("id = ? AND status = ?", input.ConversationID, conversationStatusActive).
		First(&conversationRecord).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, ErrConversationNotFound
	}
	if err != nil {
		return nil, false, fmt.Errorf("查询历史消息会话: %w", err)
	}

	var member conversationMember
	err = r.db.WithContext(ctx).
		Where(
			"conversation_id = ? AND user_id = ? AND left_at IS NULL",
			input.ConversationID,
			input.UserID,
		).
		First(&member).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, ErrNotMember
	}
	if err != nil {
		return nil, false, fmt.Errorf("校验历史消息成员: %w", err)
	}

	ascending := input.AfterSeq > 0
	query := r.db.WithContext(ctx).
		Where("conversation_id = ?", input.ConversationID)
	switch {
	case input.AfterSeq > 0:
		query = query.Where("seq > ?", input.AfterSeq).Order("seq ASC")
	case input.BeforeSeq > 0:
		query = query.Where("seq < ?", input.BeforeSeq).Order("seq DESC")
	default:
		query = query.Order("seq DESC")
	}

	records := make([]message, 0, input.Limit+1)
	if err := query.Limit(input.Limit + 1).Find(&records).Error; err != nil {
		return nil, false, fmt.Errorf("查询历史消息: %w", err)
	}

	hasMore := len(records) > input.Limit
	if hasMore {
		records = records[:input.Limit]
	}
	if !ascending {
		// 向前翻页和默认最近消息先倒序查询，再统一转成 seq 升序响应。
		slices.Reverse(records)
	}
	return records, hasMore, nil
}

func isDuplicateKey(err error) bool {
	// MySQL 1062 表示唯一键冲突；不能把连接错误或普通 SQL 错误误判成重试。
	var mysqlErr *mysql.MySQLError
	return errors.As(err, &mysqlErr) && mysqlErr.Number == 1062
}
