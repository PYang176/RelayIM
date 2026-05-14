package repository

import (
	"context"
	"ginChatProject/internal/model"
	"gorm.io/gorm"
)

type MessageRepository struct {
	db *gorm.DB
}

// NewMessageRepository 构造函数
func NewMessageRepository(db *gorm.DB) *MessageRepository {
	return &MessageRepository{db: db}
}

// Create 插入一条消息
func (r *MessageRepository) Create(ctx context.Context, msg *model.Message) error {
	return r.db.WithContext(ctx).Create(msg).Error
}

// GetOfflineMessages 获取某用户的离线消息（未读）
func (r *MessageRepository) GetOfflineMessages(ctx context.Context, userID uint) ([]*model.Message, error) {
	var messages []*model.Message
	err := r.db.WithContext(ctx).
		Where("receiver_id = ? AND status < ?", userID, model.MsgDelivered). //小于4的为未读
		Order("created_at ASC").                                             //升序排序
		Find(&messages).Error
	return messages, err
}

// UpdateStatus 更新消息状态
func (r *MessageRepository) UpdateStatus(ctx context.Context, messageID string, status model.MsgStatus) error {
	return r.db.WithContext(ctx).
		Model(&model.Message{}).
		Where("message_id = ?", messageID).
		Update("status", status).Error
}
