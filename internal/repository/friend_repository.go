package repository

import (
	"context"
	"fmt"
	"ginChatProject/internal/model"
	"ginChatProject/utils/redis_util"
	"gorm.io/gorm"
	"time"
)

type FriendRepository struct {
	db *gorm.DB
}

// NewFriendRepository 构造函数
func NewFriendRepository(db *gorm.DB) *FriendRepository {
	return &FriendRepository{db: db}
}

// CheckFriendShip 检查是否为好友(增加带缓存版本)
func (r *FriendRepository) CheckFriendShip(ctx context.Context, userID uint64, friendID uint64) (bool, error) {
	// 1. 先查 Redis
	cacheKey := fmt.Sprintf("friend:relation:%d:%d", userID, friendID)
	val, err := redis_util.RDB.Get(ctx, cacheKey).Int()
	if err == nil {
		return val == 1, nil // 命中缓存，直接返回
	}

	// 2. Redis 没有，查数据库
	var count int64
	err = r.db.WithContext(ctx).
		Model(&model.FriendRelation{}).
		Where("(user_id = ? AND friend_id = ?) OR (user_id = ? AND friend_id = ?)",
			userID, friendID, friendID, userID).
		Count(&count).Error

	if err != nil {
		return false, err
	}

	isFriend := count > 0

	// 3. 回种缓存（设置 1 小时过期，防止脏数据）
	if isFriend {
		redis_util.RDB.Set(ctx, cacheKey, 1, 1*time.Hour)
	} else {
		redis_util.RDB.Set(ctx, cacheKey, 0, 1*time.Hour)
	}

	return isFriend, nil
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
func (r *FriendRepository) UpdateRequestStatus(ctx context.Context, requestID uint, status model.FriendRequestStatus) error {
	return r.db.WithContext(ctx).
		Model(&model.FriendRequest{}).
		Where("id = ?", requestID).
		Update("status", status).Error
}

// CreateRelation 创建好友关系
func (r *FriendRepository) CreateRelation(ctx context.Context, userID, friendID uint64) error {
	// 检查是否已存在好友关系
	exists, err := r.CheckFriendShip(ctx, userID, friendID)
	if err != nil {
		return err
	}
	if exists {
		return nil // 已存在，直接返回
	}

	// 双向插入
	relations := []*model.FriendRelation{
		{UserID: userID, FriendID: friendID, Status: model.FriendNormal},
		{UserID: friendID, FriendID: userID, Status: model.FriendNormal},
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

// FindByRequestID 根据好友申请ID获取好友申请
func (r *FriendRepository) FindByRequestID(ctx context.Context, requestID uint) (*model.FriendRequest, error) {
	var req model.FriendRequest
	err := r.db.WithContext(ctx).
		Model(&model.FriendRequest{}).
		Where("id = ?", requestID).
		First(&req).Error
	return &req, err
}

// DeleteFriend 删除好友
func (r *FriendRepository) DeleteFriend(ctx context.Context, userID, friendID uint64) error {
	// 开启事务保证原子性
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 删除 userID -> friendID
		if err := tx.Where("user_id = ? AND friend_id = ?", userID, friendID).
			Delete(&model.FriendRelation{}).Error; err != nil {
			return err
		}

		// 删除 friendID -> userID
		if err := tx.Where("user_id = ? AND friend_id = ?", friendID, userID).
			Delete(&model.FriendRelation{}).Error; err != nil {
			return err
		}

		return nil
	})

}
