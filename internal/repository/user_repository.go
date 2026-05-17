package repository

import (
	"context"
	"ginChatProject/internal/model"
	"gorm.io/gorm"
)

type UserRepository struct {
	db *gorm.DB
}

// UserUpdateFields 仅包含允许更新的字段
type UserUpdateFields struct {
	Name  *string
	Phone *string
	Email *string
}

// NewUserRepository 构造函数
func NewUserRepository(db *gorm.DB) *UserRepository {
	return &UserRepository{db: db}
}

// List 获取用户列表
func (r *UserRepository) List(ctx context.Context) ([]*model.UserBasic, error) {
	var users []*model.UserBasic
	if err := r.db.WithContext(ctx).Find(&users).Error; err != nil {
		return nil, err
	}
	return users, nil
}

// Create 创建新用户
func (r *UserRepository) Create(ctx context.Context, user *model.UserBasic) error {
	return r.db.WithContext(ctx).Create(user).Error
}

// Delete 删除用户
func (r *UserRepository) Delete(ctx context.Context, user *model.UserBasic) error {
	return r.db.WithContext(ctx).Delete(user).Error
}

// Update 修改用户
func (r *UserRepository) Update(ctx context.Context, id uint, updates *model.UserBasic) error {
	return r.db.WithContext(ctx).
		Model(&model.UserBasic{}).
		Where("id = ? AND deleted_at IS NULL", id).
		Updates(updates).Error
}

// FindByID 查询指定id用户
func (r *UserRepository) FindByID(ctx context.Context, id uint) (*model.UserBasic, error) {
	var user model.UserBasic
	err := r.db.WithContext(ctx).
		Where("id = ? AND deleted_at IS NULL", id).
		First(&user).Error

	return &user, err
}

// FindByName 查询指定名称用户
func (r *UserRepository) FindByName(ctx context.Context, name string) (*model.UserBasic, error) {
	var user model.UserBasic
	err := r.db.WithContext(ctx).
		Where("name = ? AND deleted_at IS NULL", name).
		First(&user).Error
	return &user, err
}

// ExistsByName 查询用户名存不存在
func (r *UserRepository) ExistsByName(
	ctx context.Context,
	name string,
) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&model.UserBasic{}).
		Where("name = ? AND deleted_at IS NULL", name).
		Count(&count).Error

	return count > 0, err
}
