package messaging

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type attachmentCleanupCandidate struct {
	// ID 是待清理附件 Snowflake ID。
	ID uint64 `gorm:"column:id"`

	// ObjectKey 是对象存储中的内部键。
	ObjectKey string `gorm:"column:object_key"`

	// CreatedAt 是附件元数据创建时间。
	CreatedAt time.Time `gorm:"column:created_at"`
}

// createAttachment 创建附件并在事务内维护关联状态
func (r *Repository) createAttachment(
	ctx context.Context,
	record *attachment,
) error {
	if err := r.db.WithContext(ctx).Create(record).Error; err != nil {
		return fmt.Errorf("创建附件元数据: %w", err)
	}
	return nil
}

// finalizeAttachment 把待确认附件切换为可用状态，并记录最终确认时间
func (r *Repository) finalizeAttachment(
	ctx context.Context,
	attachmentID uint64,
	finalizedAt time.Time,
) error {
	result := r.db.WithContext(ctx).
		Model(&attachment{}).
		Where("id = ? AND status = ?", attachmentID, attachmentStatusPending).
		Updates(map[string]any{
			"status":       attachmentStatusReady,
			"finalized_at": finalizedAt,
		})
	if result.Error != nil {
		return fmt.Errorf("确认附件可用状态: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return ErrAttachmentNotFound
	}
	return nil
}

// deleteAttachmentMetadata 删除附件Metadata并遵循关联约束
func (r *Repository) deleteAttachmentMetadata(
	ctx context.Context,
	attachmentID uint64,
) error {
	return r.db.WithContext(ctx).
		Where("id = ?", attachmentID).
		Delete(&attachment{}).Error
}

// claimOrphanAttachments 领取超过 TTL 且未绑定消息的孤儿附件
func (r *Repository) claimOrphanAttachments(
	ctx context.Context,
	now time.Time,
	pendingTTL time.Duration,
	lease time.Duration,
	limit int,
) ([]attachmentCleanupCandidate, error) {
	var candidates []attachmentCleanupCandidate
	cutoff := now.Add(-pendingTTL)
	staleLease := now.Add(-lease)
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Table("attachments AS a").
			Select("a.id, a.object_key, a.created_at").
			Where(
				"((a.status IN ? AND a.created_at < ?) OR "+
					"(a.status = ? AND a.cleanup_started_at < ?)) "+
					"AND NOT EXISTS ("+
					"SELECT 1 FROM messages AS m WHERE m.attachment_id = a.id)",
				[]int{
					attachmentStatusPending,
					attachmentStatusDeleted,
				},
				cutoff,
				attachmentStatusCleaning,
				staleLease,
			).
			Order("a.created_at ASC").
			Limit(limit).
			Clauses(clause.Locking{
				Strength: "UPDATE",
				Options:  "SKIP LOCKED",
			}).
			Find(&candidates).Error; err != nil {
			return fmt.Errorf("查询孤儿附件: %w", err)
		}
		if len(candidates) == 0 {
			return nil
		}
		ids := make([]uint64, 0, len(candidates))
		for _, candidate := range candidates {
			ids = append(ids, candidate.ID)
		}
		if err := tx.Model(&attachment{}).
			Where("id IN ?", ids).
			Updates(map[string]any{
				"status":             attachmentStatusCleaning,
				"cleanup_started_at": now,
			}).Error; err != nil {
			return fmt.Errorf("领取孤儿附件: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return candidates, nil
}

// markAttachmentCleanupFailed 把清理失败附件放回可重试状态
func (r *Repository) markAttachmentCleanupFailed(
	ctx context.Context,
	attachmentID uint64,
) error {
	result := r.db.WithContext(ctx).
		Model(&attachment{}).
		Where("id = ? AND status = ?", attachmentID, attachmentStatusCleaning).
		Updates(map[string]any{
			"status":             attachmentStatusDeleted,
			"cleanup_started_at": nil,
		})
	if result.Error != nil {
		return fmt.Errorf("标记附件清理失败: %w", result.Error)
	}
	return nil
}

// deleteCleanedAttachmentMetadata 删除已经移除对象的附件元数据。
func (r *Repository) deleteCleanedAttachmentMetadata(
	ctx context.Context,
	attachmentID uint64,
) error {
	result := r.db.WithContext(ctx).
		Where("id = ? AND status = ?", attachmentID, attachmentStatusCleaning).
		Delete(&attachment{})
	if result.Error != nil {
		return fmt.Errorf("删除清理后附件元数据: %w", result.Error)
	}
	return nil
}

// CountOrphanAttachments 统计当前满足清理条件的孤儿附件数量。
func (r *Repository) CountOrphanAttachments(
	ctx context.Context,
	now time.Time,
	pendingTTL time.Duration,
	lease time.Duration,
) (int64, error) {
	var count int64
	if err := r.db.WithContext(ctx).
		Table("attachments AS a").
		Where(
			"((a.status IN ? AND a.created_at < ?) OR "+
				"(a.status = ? AND a.cleanup_started_at < ?)) "+
				"AND NOT EXISTS ("+
				"SELECT 1 FROM messages AS m WHERE m.attachment_id = a.id)",
			[]int{
				attachmentStatusPending,
				attachmentStatusDeleted,
			},
			now.Add(-pendingTTL),
			attachmentStatusCleaning,
			now.Add(-lease),
		).
		Count(&count).Error; err != nil {
		return 0, fmt.Errorf("统计孤儿附件: %w", err)
	}
	return count, nil
}

// findAccessibleAttachment 按附件归属或会话成员关系校验访问权限
func (r *Repository) findAccessibleAttachment(
	ctx context.Context,
	userID uint64,
	attachmentID uint64,
) (*attachment, error) {
	var record attachment
	if err := r.db.WithContext(ctx).
		Where("id = ? AND status = ?", attachmentID, attachmentStatusReady).
		First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrAttachmentNotFound
		}
		return nil, fmt.Errorf("查询附件: %w", err)
	}
	if record.OwnerID == userID {
		return &record, nil
	}

	var memberCount int64
	if err := r.db.WithContext(ctx).
		Table("messages AS m").
		Joins(
			"JOIN conversation_members AS cm "+
				"ON cm.conversation_id = m.conversation_id",
		).
		Where(
			"m.attachment_id = ? AND cm.user_id = ? AND cm.left_at IS NULL "+
				"AND m.recalled_at IS NULL",
			attachmentID,
			userID,
		).
		Count(&memberCount).Error; err != nil {
		return nil, fmt.Errorf("校验附件会话权限: %w", err)
	}
	if memberCount != 1 {
		return nil, ErrAttachmentNotFound
	}
	return &record, nil
}

// usableAttachment 校验附件归属、状态和未绑定约束，返回可绑定附件
func usableAttachment(
	ctx context.Context,
	tx *gorm.DB,
	ownerID uint64,
	attachmentID uint64,
) (*attachment, error) {
	var record attachment
	if err := tx.WithContext(ctx).
		Where(
			"id = ? AND owner_id = ? AND status = ?",
			attachmentID,
			ownerID,
			attachmentStatusReady,
		).
		First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrInvalidAttachment
		}
		return nil, fmt.Errorf("校验消息附件: %w", err)
	}

	var usedCount int64
	if err := tx.WithContext(ctx).
		Model(&message{}).
		Where("attachment_id = ?", attachmentID).
		Count(&usedCount).Error; err != nil {
		return nil, fmt.Errorf("检查附件消息绑定: %w", err)
	}
	if usedCount > 0 {
		return nil, ErrAttachmentAlreadyUsed
	}
	return &record, nil
}
