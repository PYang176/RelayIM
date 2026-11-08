package messaging

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
)

func (r *Repository) createAttachment(
	ctx context.Context,
	record *attachment,
) error {
	if err := r.db.WithContext(ctx).Create(record).Error; err != nil {
		return fmt.Errorf("创建附件元数据: %w", err)
	}
	return nil
}

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

func (r *Repository) deleteAttachmentMetadata(
	ctx context.Context,
	attachmentID uint64,
) error {
	return r.db.WithContext(ctx).
		Where("id = ?", attachmentID).
		Delete(&attachment{}).Error
}

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
