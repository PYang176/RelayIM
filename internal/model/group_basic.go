package model

import (
	"gorm.io/gorm"
)

// GroupStatus 群状态
type GroupStatus int

const (
	GroupActive    GroupStatus = 1 // 正常
	GroupDissolved GroupStatus = 2 // 已解散
)

// GroupType 群类型
type GroupType int

const (
	GroupNormal GroupType = 1 // 普通群
	GroupSystem GroupType = 2 // 系统群
)

// GroupBasic 群信息表
type GroupBasic struct {
	gorm.Model

	GroupID string `gorm:"uniqueIndex;size:64"` // 业务群ID
	Name    string `gorm:"size:100"`
	Avatar  string //头像
	OwnerID uint64 `gorm:"index"` // 群主

	GroupType GroupType   `gorm:"type:tinyint"`
	Status    GroupStatus `gorm:"type:tinyint;default:1"`
}

// TableName 表名
func (table *GroupBasic) TableName() string {
	return "group_basic"
}
