package conversation

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// UpdateGroupAvatar 校验群主权限并更新头像地址和对象键。
func (r *Repository) UpdateGroupAvatar(
	ctx context.Context,
	actorID uint64,
	conversationID uint64,
	objectKey *string,
	avatarURL string,
) (string, error) {
	var previousObjectKey string
	now := time.Now().UTC()
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		record, err := lockOwnedGroup(ctx, tx, actorID, conversationID)
		if err != nil {
			return err
		}
		previousObjectKey = stringValue(record.AvatarObjectKey)
		if err := tx.Model(&conversation{}).
			Where("id = ? AND status = ?", conversationID, conversationStatusOpen).
			Updates(map[string]any{
				"avatar_object_key": objectKey,
				"avatar_url":        nullableString(avatarURL),
				"updated_at":        now,
			}).Error; err != nil {
			return fmt.Errorf("更新群头像: %w", err)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return previousObjectKey, nil
}

// FindGroupAvatarObjectKey 查询群头像对象键并拒绝私聊和已解散群。
func (r *Repository) FindGroupAvatarObjectKey(
	ctx context.Context,
	conversationID uint64,
) (string, error) {
	var record struct {
		ConversationType int
		AvatarObjectKey  *string
	}
	err := r.db.WithContext(ctx).
		Model(&conversation{}).
		Select("conversation_type, avatar_object_key").
		Where(
			"id = ? AND status = ?",
			conversationID,
			conversationStatusOpen,
		).
		Take(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", ErrConversationNotFound
	}
	if err != nil {
		return "", fmt.Errorf("查询群头像对象键: %w", err)
	}
	if record.ConversationType != conversationTypeGroup {
		return "", ErrNotGroup
	}
	if record.AvatarObjectKey == nil || *record.AvatarObjectKey == "" {
		return "", ErrGroupAvatarNotFound
	}
	return *record.AvatarObjectKey, nil
}
