package repository

import (
	"context"
	"ginChatProject/internal/model"
	"gorm.io/gorm"
)

type GroupRepository struct {
	db *gorm.DB
}

// NewGroupRepository 构造函数
func NewGroupRepository(db *gorm.DB) *GroupRepository {
	return &GroupRepository{db: db}
}

// Create 创建群聊
func (r *GroupRepository) Create(ctx context.Context, group *model.GroupBasic) error {
	return r.db.WithContext(ctx).Create(group).Error
}

// AddMember 添加群成员
func (r *GroupRepository) AddMember(ctx context.Context, member *model.GroupMember) error {
	return r.db.WithContext(ctx).Create(member).Error
}

// RemoveMember 移除群成员
func (r *GroupRepository) RemoveMember(ctx context.Context, groupID uint64, userID uint64) error {
	return r.db.WithContext(ctx).
		Where("group_id = ? AND user_id = ?", groupID, userID).
		Delete(&model.GroupMember{}).Error
}

// GetGroupByID 根据群ID获取群信息
func (r *GroupRepository) GetGroupByID(ctx context.Context, groupID uint64) (*model.GroupBasic, error) {
	var group model.GroupBasic
	err := r.db.WithContext(ctx).
		Where("group_id = ?", groupID).
		Model(&model.GroupBasic{}).
		First(&group).Error
	return &group, err
}

// GetGroupMembers 获取群成员列表
func (r *GroupRepository) GetGroupMembers(ctx context.Context, groupID uint64) ([]*model.GroupMember, error) {
	var members []*model.GroupMember
	err := r.db.WithContext(ctx).
		Model(&model.GroupMember{}).
		Where("group_id = ?", groupID).
		Find(&members).Error
	return members, err
}

// CheckMembership 检查用户是否在群中
func (r *GroupRepository) CheckMembership(ctx context.Context, groupID, userID uint64) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&model.GroupMember{}).
		Where("group_id = ? AND user_id = ?", groupID, userID).
		Count(&count).Error
	return count > 0, err
}

// GetUserGroups 获取用户加入的所有群
func (r *GroupRepository) GetUserGroups(ctx context.Context, userID uint64) ([]*model.GroupBasic, error) {
	var groups []*model.GroupBasic
	err := r.db.WithContext(ctx).
		Joins("JOIN group_member ON group_member.group_id = group_basic.id").
		Where("group_member.user_id = ?", userID).
		Find(&groups).Error
	return groups, err
}
