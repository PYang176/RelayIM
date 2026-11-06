package model

import (
	"gorm.io/gorm"
)

// FriendStatus 好友状态
type FriendStatus int

const (
	FriendNormal  FriendStatus = 1 // 正常
	FriendBlocked FriendStatus = 2 // 拉黑
)

// FriendRelation 好友关系表
type FriendRelation struct {
	gorm.Model
	// 联合唯一索引（防止重复好友）
	//UNIQUE KEY uk_user_friend (user_id, friend_id)
	//UserID   uint64 `gorm:"uniqueIndex:uk_user_friend"` // 用户
	//FriendID uint64 `gorm:"uniqueIndex:uk_user_friend"` // 好友

	// UserID 是好友关系方向上的用户 ID。
	UserID uint64 `gorm:"index"`
	// FriendID 是该用户对应的好友 ID。
	FriendID uint64 `gorm:"index"`

	// Remark 是该用户为好友设置的备注名。
	Remark string

	// Status 表示好友关系正常或已拉黑。
	Status FriendStatus `gorm:"type:tinyint;default:1"`
}

// TableName 表名
func (table *FriendRelation) TableName() string {
	return "friend_relation"
}
