package messaging

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
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

type sendMessageInput struct {
	ConversationID   uint64
	SenderID         uint64
	ClientMessageID  string
	MessageType      int
	Content          string
	ReplyToMessageID *uint64
	AttachmentID     *uint64
}

type listMessagesInput struct {
	ConversationID uint64
	UserID         uint64
	AfterSeq       uint64
	BeforeSeq      uint64
	Limit          int
}

type searchMessagesInput struct {
	ConversationID uint64
	UserID         uint64
	Query          string
	BeforeSeq      uint64
	Limit          int
}

type readStateRecord struct {
	UserID      uint64 `gorm:"column:user_id"`
	LastReadSeq uint64 `gorm:"column:last_read_seq"`
}

type deliverySummaryRecord struct {
	DeliveredDevices uint64 `gorm:"column:delivered_devices"`
	DeliveredUsers   uint64 `gorm:"column:delivered_users"`
}

// sendMessage 在单个事务中保存消息、Outbox 和会话序号。
func (r *Repository) sendMessage(
	ctx context.Context,
	input sendMessageInput,
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
		MessageType:      input.MessageType,
		Content:          input.Content,
		ReplyToMessageID: input.ReplyToMessageID,
		AttachmentID:     input.AttachmentID,
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

		// 拉黑是用户对级规则，历史私聊仍然可读，但不能继续写入新消息。
		if conversationRecord.ConversationType == conversationTypeDirect {
			if conversationRecord.DirectLowUserID == nil ||
				conversationRecord.DirectHighUserID == nil {
				return fmt.Errorf("私聊会话缺少用户对")
			}
			peerUserID := *conversationRecord.DirectLowUserID
			if peerUserID == input.SenderID {
				peerUserID = *conversationRecord.DirectHighUserID
			}
			var blockedCount int64
			if err := tx.Table("user_blocks").
				Where(
					"(blocker_id = ? AND blocked_id = ?) OR "+
						"(blocker_id = ? AND blocked_id = ?)",
					input.SenderID,
					peerUserID,
					peerUserID,
					input.SenderID,
				).
				Count(&blockedCount).Error; err != nil {
				return fmt.Errorf("校验私聊拉黑关系: %w", err)
			}
			if blockedCount > 0 {
				return ErrUserBlocked
			}
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

		var attachmentRecord *attachment
		if input.AttachmentID != nil {
			attachmentRecord, err = usableAttachment(
				ctx,
				tx,
				input.SenderID,
				*input.AttachmentID,
			)
			if err != nil {
				return err
			}
			if input.MessageType == messageTypeImage &&
				!strings.HasPrefix(attachmentRecord.MIMEType, "image/") {
				return ErrInvalidAttachment
			}
		}

		// 当前事务持有会话行锁，LastSeq 在读取和更新之间不会被其他
		// 发送者修改，因此当前会话的 seq 能保持严格连续。
		msg.Seq = conversationRecord.LastSeq + 1
		if err := tx.Create(msg).Error; err != nil {
			return fmt.Errorf("保存消息: %w", err)
		}

		// 发送者本人已经知道刚提交的消息，必须在同一事务推进自己的
		// 已读游标，否则下一次会话列表会把本人发送的消息算成未读。
		if err := tx.Model(&conversationMember{}).
			Where(
				"conversation_id = ? AND user_id = ? AND left_at IS NULL",
				input.ConversationID,
				input.SenderID,
			).
			Update(
				"last_read_seq",
				gorm.Expr("GREATEST(last_read_seq, ?)", msg.Seq),
			).Error; err != nil {
			return fmt.Errorf("推进发送者已读游标: %w", err)
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
		attachmentID := ""
		attachmentName := ""
		if attachmentRecord != nil {
			attachmentID = strconv.FormatUint(attachmentRecord.ID, 10)
			attachmentName = attachmentRecord.OriginalName
		}
		payload, err := json.Marshal(messageCreatedPayload{
			MessageID:      strconv.FormatUint(msg.ID, 10),
			ConversationID: strconv.FormatUint(msg.ConversationID, 10),
			SenderID:       strconv.FormatUint(msg.SenderID, 10),
			Seq:            msg.Seq,
			MessageType:    msg.MessageType,
			AttachmentID:   attachmentID,
			AttachmentName: attachmentName,
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
	// 同一附件被并发绑定也会触发 1062。此时事务已回滚，先按客户端
	// 幂等键读取先提交的原消息；找不到时说明冲突来自附件唯一约束。
	if isDuplicateKey(err) {
		existing, findErr := r.findByClientMessageID(ctx, input.SenderID, input.ClientMessageID)
		if findErr == nil {
			if existing.ConversationID != input.ConversationID {
				return nil, false, ErrClientMessageConflict
			}
			return existing, true, nil
		}
		if !errors.Is(findErr, gorm.ErrRecordNotFound) {
			return nil, false, fmt.Errorf("读取重复消息: %w", findErr)
		}
		if input.AttachmentID != nil {
			return nil, false, ErrAttachmentAlreadyUsed
		}
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
	if err := r.ensureActiveMember(
		ctx,
		input.ConversationID,
		input.UserID,
	); err != nil {
		return nil, false, err
	}

	ascending := input.AfterSeq > 0
	query := r.db.WithContext(ctx).
		Table("messages AS m").
		Select(
			"m.*, a.original_name AS attachment_name, "+
				"a.object_key AS attachment_object_key",
		).
		Joins("LEFT JOIN attachments AS a ON a.id = m.attachment_id").
		Where("m.conversation_id = ?", input.ConversationID)
	switch {
	case input.AfterSeq > 0:
		query = query.Where("m.seq > ?", input.AfterSeq).Order("m.seq ASC")
	case input.BeforeSeq > 0:
		query = query.Where("m.seq < ?", input.BeforeSeq).Order("m.seq DESC")
	default:
		query = query.Order("m.seq DESC")
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

func (r *Repository) ensureActiveMember(
	ctx context.Context,
	conversationID uint64,
	userID uint64,
) error {
	var conversationRecord conversation
	err := r.db.WithContext(ctx).
		Where("id = ? AND status = ?", conversationID, conversationStatusActive).
		First(&conversationRecord).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrConversationNotFound
	}
	if err != nil {
		return fmt.Errorf("查询消息会话: %w", err)
	}

	var member conversationMember
	err = r.db.WithContext(ctx).
		Where(
			"conversation_id = ? AND user_id = ? AND left_at IS NULL",
			conversationID,
			userID,
		).
		First(&member).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotMember
	}
	if err != nil {
		return fmt.Errorf("校验消息成员: %w", err)
	}
	return nil
}

// searchMessages 在单个会话内使用 ngram 全文索引检索消息正文。
func (r *Repository) searchMessages(
	ctx context.Context,
	input searchMessagesInput,
) ([]message, bool, error) {
	if err := r.ensureActiveMember(
		ctx,
		input.ConversationID,
		input.UserID,
	); err != nil {
		return nil, false, err
	}

	query := r.db.WithContext(ctx).
		Table("messages AS m").
		Select(
			"m.id, m.conversation_id, m.seq, m.sender_id, "+
				"m.message_type, m.content, m.created_at, "+
				"COALESCE(up.nickname, u.username, '未知用户') "+
				"AS sender_name",
		).
		Joins("JOIN users AS u ON u.id = m.sender_id").
		Joins("LEFT JOIN user_profiles AS up ON up.user_id = u.id").
		Where(
			"m.conversation_id = ? AND m.recalled_at IS NULL "+
				"AND MATCH(m.content) AGAINST "+
				"(? IN NATURAL LANGUAGE MODE)",
			input.ConversationID,
			input.Query,
		)
	if input.BeforeSeq > 0 {
		query = query.Where("m.seq < ?", input.BeforeSeq)
	}

	records := make([]message, 0, input.Limit+1)
	if err := query.
		Order("m.seq DESC").
		Limit(input.Limit + 1).
		Find(&records).Error; err != nil {
		return nil, false, fmt.Errorf("搜索会话消息: %w", err)
	}

	hasMore := len(records) > input.Limit
	if hasMore {
		records = records[:input.Limit]
	}
	return records, hasMore, nil
}

// recallMessage 在事务中校验权限和撤回时间窗，并写入可靠实时事件。
func (r *Repository) recallMessage(
	ctx context.Context,
	conversationID uint64,
	userID uint64,
	messageID uint64,
	now time.Time,
) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var conversationRecord conversation
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", conversationID).
			First(&conversationRecord).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrConversationNotFound
			}
			return fmt.Errorf("锁定撤回消息会话: %w", err)
		}
		if conversationRecord.Status != conversationStatusActive {
			return ErrConversationClosed
		}

		var member conversationMember
		if err := tx.Where(
			"conversation_id = ? AND user_id = ? AND left_at IS NULL",
			conversationID,
			userID,
		).First(&member).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotMember
			}
			return fmt.Errorf("校验撤回操作人: %w", err)
		}

		var record message
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where(
				"id = ? AND conversation_id = ?",
				messageID,
				conversationID,
			).
			First(&record).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrMessageNotFound
			}
			return fmt.Errorf("锁定待撤回消息: %w", err)
		}
		if record.RecalledAt != nil {
			return nil
		}

		// 普通发送者只能撤回自己两分钟内的消息；群主和管理员可以治理
		// 群内一天内的消息。会话行锁保证并发撤回和发送不会读到半状态。
		isSender := record.SenderID == userID
		isGroupManager := conversationRecord.ConversationType == conversationTypeGroup &&
			member.Role >= groupRoleAdmin
		canRecallAsSender := isSender &&
			!now.After(record.CreatedAt.Add(senderRecallWindow))
		canRecallAsManager := isGroupManager &&
			!now.After(record.CreatedAt.Add(managerRecallWindow))
		if !canRecallAsSender && !canRecallAsManager {
			if !isSender && !isGroupManager {
				return ErrRecallForbidden
			}
			return ErrRecallExpired
		}

		if err := tx.Model(&message{}).
			Where("id = ?", messageID).
			Updates(map[string]any{
				"recalled_at": now,
				"recalled_by": userID,
			}).Error; err != nil {
			return fmt.Errorf("更新消息撤回状态: %w", err)
		}

		payload, err := json.Marshal(messageRecalledPayload{
			MessageID:      strconv.FormatUint(record.ID, 10),
			ConversationID: strconv.FormatUint(conversationID, 10),
			SenderID:       strconv.FormatUint(record.SenderID, 10),
			RecalledBy:     strconv.FormatUint(userID, 10),
			Seq:            record.Seq,
			RecalledAt:     now,
		})
		if err != nil {
			return fmt.Errorf("序列化消息撤回事件: %w", err)
		}
		if err := tx.Create(&outboxEvent{
			EventID:       uint64(snowflake.Generate()),
			AggregateType: "conversation",
			AggregateID:   conversationID,
			EventType:     "message.recalled.v1",
			Payload:       string(payload),
			Status:        outboxStatusPending,
			AvailableAt:   now,
			CreatedAt:     now,
		}).Error; err != nil {
			return fmt.Errorf("保存消息撤回事件: %w", err)
		}
		return nil
	})
}

// acknowledgeMessage 记录消息到具体设备的幂等送达回执。
func (r *Repository) acknowledgeMessage(
	ctx context.Context,
	userID uint64,
	deviceID uint64,
	messageID uint64,
) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var deviceRecord device
		if err := tx.Where(
			"id = ? AND user_id = ? AND revoked_at IS NULL",
			deviceID,
			userID,
		).First(&deviceRecord).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrDeviceUnavailable
			}
			return fmt.Errorf("校验 ACK 设备: %w", err)
		}

		var memberCount int64
		if err := tx.Table("messages AS m").
			Joins(
				"JOIN conversation_members AS cm "+
					"ON cm.conversation_id = m.conversation_id",
			).
			Joins(
				"JOIN conversations AS c ON c.id = m.conversation_id",
			).
			Where(
				"m.id = ? AND cm.user_id = ? AND cm.left_at IS NULL "+
					"AND c.status = ?",
				messageID,
				userID,
				conversationStatusActive,
			).
			Count(&memberCount).Error; err != nil {
			return fmt.Errorf("校验 ACK 消息权限: %w", err)
		}
		if memberCount != 1 {
			return ErrNotMember
		}

		if err := tx.Exec(
			"INSERT IGNORE INTO message_deliveries "+
				"(message_id, device_id, delivered_at) VALUES (?, ?, ?)",
			messageID,
			deviceID,
			time.Now().UTC(),
		).Error; err != nil {
			return fmt.Errorf("保存消息送达回执: %w", err)
		}
		return nil
	})
}

// markConversationRead 单调推进成员的已读游标。
func (r *Repository) markConversationRead(
	ctx context.Context,
	userID uint64,
	conversationID uint64,
	seq uint64,
) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var conversationRecord conversation
		if err := tx.Where(
			"id = ? AND status = ?",
			conversationID,
			conversationStatusActive,
		).
			First(&conversationRecord).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrConversationNotFound
			}
			return fmt.Errorf("查询已读会话: %w", err)
		}
		if seq > conversationRecord.LastSeq {
			return ErrInvalidReadSequence
		}

		var member conversationMember
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where(
				"conversation_id = ? AND user_id = ? AND left_at IS NULL",
				conversationID,
				userID,
			).
			First(&member).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotMember
			}
			return fmt.Errorf("查询已读成员: %w", err)
		}
		if member.LastReadSeq >= seq {
			return nil
		}
		if err := tx.Model(&conversationMember{}).
			Where("conversation_id = ? AND user_id = ?", conversationID, userID).
			Update("last_read_seq", seq).Error; err != nil {
			return fmt.Errorf("更新成员已读序号: %w", err)
		}
		return nil
	})
}

// listReadStates 返回会话有效成员的已读游标。
func (r *Repository) listReadStates(
	ctx context.Context,
	userID uint64,
	conversationID uint64,
) ([]ReadStateResult, error) {
	var memberCount int64
	if err := r.db.WithContext(ctx).
		Table("conversation_members AS cm").
		Joins("JOIN conversations AS c ON c.id = cm.conversation_id").
		Where(
			"cm.conversation_id = ? AND cm.user_id = ? "+
				"AND cm.left_at IS NULL AND c.status = ?",
			conversationID,
			userID,
			conversationStatusActive,
		).
		Count(&memberCount).Error; err != nil {
		return nil, fmt.Errorf("校验已读状态查询权限: %w", err)
	}
	if memberCount != 1 {
		return nil, ErrNotMember
	}

	var records []readStateRecord
	if err := r.db.WithContext(ctx).
		Table("conversation_members").
		Select("user_id, last_read_seq").
		Where("conversation_id = ? AND left_at IS NULL", conversationID).
		Order("user_id ASC").
		Scan(&records).Error; err != nil {
		return nil, fmt.Errorf("查询成员已读状态: %w", err)
	}

	results := make([]ReadStateResult, 0, len(records))
	for _, record := range records {
		results = append(results, ReadStateResult{
			UserID:      record.UserID,
			LastReadSeq: record.LastReadSeq,
		})
	}
	return results, nil
}

// getDeliverySummary 统计消息已确认送达的设备和用户数量。
func (r *Repository) getDeliverySummary(
	ctx context.Context,
	userID uint64,
	conversationID uint64,
	messageID uint64,
) (DeliverySummary, error) {
	var memberCount int64
	if err := r.db.WithContext(ctx).
		Table("conversation_members AS cm").
		Joins("JOIN conversations AS c ON c.id = cm.conversation_id").
		Where(
			"cm.conversation_id = ? AND cm.user_id = ? "+
				"AND cm.left_at IS NULL AND c.status = ?",
			conversationID,
			userID,
			conversationStatusActive,
		).
		Count(&memberCount).Error; err != nil {
		return DeliverySummary{}, fmt.Errorf("校验送达状态查询权限: %w", err)
	}
	if memberCount != 1 {
		return DeliverySummary{}, ErrNotMember
	}

	var messageCount int64
	if err := r.db.WithContext(ctx).Model(&message{}).
		Where(
			"id = ? AND conversation_id = ?",
			messageID,
			conversationID,
		).
		Count(&messageCount).Error; err != nil {
		return DeliverySummary{}, fmt.Errorf("校验送达状态消息: %w", err)
	}
	if messageCount != 1 {
		return DeliverySummary{}, ErrMessageNotFound
	}

	var summary deliverySummaryRecord
	if err := r.db.WithContext(ctx).
		Table("message_deliveries AS md").
		Select(
			"COUNT(*) AS delivered_devices, "+
				"COUNT(DISTINCT d.user_id) AS delivered_users",
		).
		Joins("JOIN devices AS d ON d.id = md.device_id").
		Where("md.message_id = ?", messageID).
		Scan(&summary).Error; err != nil {
		return DeliverySummary{}, fmt.Errorf("统计消息送达状态: %w", err)
	}
	return DeliverySummary{
		DeliveredDevices: summary.DeliveredDevices,
		DeliveredUsers:   summary.DeliveredUsers,
	}, nil
}

func isDuplicateKey(err error) bool {
	// MySQL 1062 表示唯一键冲突；不能把连接错误或普通 SQL 错误误判成重试。
	var mysqlErr *mysql.MySQLError
	return errors.As(err, &mysqlErr) && mysqlErr.Number == 1062
}
