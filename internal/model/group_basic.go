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

	// GroupID 是旧群模型的业务群 ID。
	GroupID string `gorm:"uniqueIndex;size:64"`
	// Name 是群名称。
	Name string `gorm:"size:100"`
	// Avatar 是群头像地址。
	Avatar string
	// OwnerID 是群主用户 ID。
	OwnerID uint64 `gorm:"index"`

	// GroupType 表示普通群或系统群。
	GroupType GroupType `gorm:"type:tinyint"`
	// Status 表示群正常或已解散。
	Status GroupStatus `gorm:"type:tinyint;default:1"`
}

// TableName 表名
func (table *GroupBasic) TableName() string {
	return "group_basic"
}
