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

	GroupID uint64 `gorm:"uniqueIndex:uk_group_user"`
	UserID  uint64 `gorm:"uniqueIndex:uk_group_user"`
	//GroupID uint64 `gorm:"index"`
	//UserID  uint64 `gorm:"index"`

	Role GroupRole `gorm:"type:tinyint;default:1"`

	Nickname string // 群内昵称
	JoinTime time.Time
}

// TableName 表名
func (table *GroupMember) TableName() string {
	return "group_member"
}
