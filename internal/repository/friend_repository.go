package repository

import (
	"context"
	"ginChatProject/internal/model"
	"gorm.io/gorm"
)

type FriendRepository struct {
	db *gorm.DB
}

// NewFriendRepository 构造函数
func NewFriendRepository(db *gorm.DB) *FriendRepository {
	return &FriendRepository{db: db}
}

// CheckFriendShip 检查是否为好友
func (r *FriendRepository) CheckFriendShip(ctx context.Context, userID uint64, friendID uint64) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&model.FriendRelation{}).
		Where("(user_id = ? AND friend_id = ?) OR (user_id = ? AND friend_id = ?)", userID, friendID, friendID, userID).
		Count(&count).Error
	return count > 0, err
}

// CreateRequest 创建好友申请
func (r *FriendRepository) CreateRequest(ctx context.Context, req *model.FriendRequest) error {
	return r.db.WithContext(ctx).Create(req).Error
}

// GetReceiveRequests 获取收到的好友申请
func (r *FriendRepository) GetReceiveRequests(ctx context.Context, userID uint) ([]*model.FriendRequest, error) {
	var friendRequestList []*model.FriendRequest
	err := r.db.WithContext(ctx).
		Model(model.FriendRequest{}).
		Where("to_user_id = ?", userID).
		Order("created_at DESC"). //按创造时间排序
		Find(&friendRequestList).Error
	return friendRequestList, err
}

// GetSentRequests 获取发出的好友申请
func (r *FriendRepository) GetSentRequests(ctx context.Context, userID uint) ([]*model.FriendRequest, error) {
	var friendRequestList []*model.FriendRequest
	err := r.db.WithContext(ctx).
		Model(model.FriendRequest{}).
		Where("from_user_id = ?", userID).
		Order("created_at DESC"). //按创造时间排序
		Find(&friendRequestList).Error
	return friendRequestList, err
}

// UpdateRequestStatus 变更申请状态
func (r *FriendRepository) UpdateRequestStatus(ctx context.Context, requestID uint64, status model.FriendRequestStatus) error {
	return r.db.WithContext(ctx).
		Model(&model.FriendRequest{}).
		Where("id = ?", requestID).
		Update("status", status).Error
}

// CreateRelation 创建好友关系
func (r *FriendRepository) CreateRelation(ctx context.Context, userID, friendID uint64) error {
	// 双向插入
	relations := []*model.FriendRelation{
		{UserID: userID, FriendID: friendID},
		{UserID: friendID, FriendID: userID},
	}
	return r.db.WithContext(ctx).Create(&relations).Error
}

// ListFriends 获取好友列表
func (r *FriendRepository) ListFriends(ctx context.Context, userID uint64) ([]*model.FriendRelation, error) {
	var friends []*model.FriendRelation
	err := r.db.WithContext(ctx).
		Model(&model.FriendRelation{}).
		Where("user_id = ?", userID).
		Find(&friends).Error
	return friends, err
}

// UserExists 查询用户是否存在
func (r *FriendRepository) UserExists(ctx context.Context, userID uint64) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&model.UserBasic{}).
		Where("id = ?", userID).
		Count(&count).Error
	return count > 0, err
}

// HasPendingRequest 查询是否有重复好友申请
func (r *FriendRepository) HasPendingRequest(ctx context.Context, fromUserID, toUserID uint64) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&model.FriendRequest{}).
		Where(
			"from_user_id = ? AND to_user_id = ? AND status = ?",
			fromUserID, toUserID, model.FriendRequestPending,
		).
		Count(&count).Error
	return count > 0, err
}
