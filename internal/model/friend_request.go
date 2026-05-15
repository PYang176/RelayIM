package model

import "gorm.io/gorm"

// FriendRequestStatus 消息类型
type FriendRequestStatus int

const (
	FriendRequestPending  FriendRequestStatus = 1 // 待处理
	FriendRequestAccepted FriendRequestStatus = 2 // 已同意
	FriendRequestRejected FriendRequestStatus = 3 // 已拒绝
)

type FriendRequest struct {
	gorm.Model
	FromUserID uint64              `gorm:"index"`     // 申请人
	ToUserID   uint64              `gorm:"index"`     // 被申请人
	Status     FriendRequestStatus `gorm:"default:1"` // 1待处理 2已同意 3已拒绝
	Remark     string              //备注
}

func (table *FriendRequest) TableName() string {
	return "friend_request"
}
