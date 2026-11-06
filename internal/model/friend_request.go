package model

import "gorm.io/gorm"

// FriendRequestStatus 好友申请状态
type FriendRequestStatus int

const (
	FriendRequestPending  FriendRequestStatus = 1 // 待处理
	FriendRequestAccepted FriendRequestStatus = 2 // 已同意
	FriendRequestRejected FriendRequestStatus = 3 // 已拒绝
)

type FriendRequest struct {
	gorm.Model
	// FromUserID 是发起好友申请的用户 ID。
	FromUserID uint64 `gorm:"index"`
	// ToUserID 是接收好友申请的用户 ID。
	ToUserID uint64 `gorm:"index"`
	// Status 表示待处理、已同意或已拒绝。
	Status FriendRequestStatus `gorm:"default:1"`
	// Remark 是申请附带的说明。
	Remark string
}

func (table *FriendRequest) TableName() string {
	return "friend_request"
}
