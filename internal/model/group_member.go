package model

import (
	"gorm.io/gorm"
	"time"
)

// GroupRole 群成员角色
type GroupRole int

const (
	GroupMemberRole GroupRole = 1 // 普通成员
	GroupAdminRole  GroupRole = 2 // 管理员
	GroupOwnerRole  GroupRole = 3 // 群主
)

// GroupMember 群成员表
type GroupMember struct {
	gorm.Model

	//GroupID uint64 `gorm:"uniqueIndex:uk_group_user"`
	//UserID  uint64 `gorm:"uniqueIndex:uk_group_user"`
	// GroupID 是旧群模型的群 ID。
	GroupID uint64 `gorm:"index"`
	// UserID 是群成员用户 ID。
	UserID uint64 `gorm:"index"`

	// Role 表示成员、管理员或群主。
	Role GroupRole `gorm:"type:tinyint;default:1"`

	// Nickname 是该用户在群内展示的昵称。
	Nickname string
	// JoinTime 是用户加入群聊的时间。
	JoinTime time.Time
}

// TableName 表名
func (table *GroupMember) TableName() string {
	return "group_member"
}
