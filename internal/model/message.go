package model

import (
	"gorm.io/gorm"
	"time"
)

// MsgType 消息类型
type MsgType int

const (
	MsgText  MsgType = 1 // 文字
	MsgImage MsgType = 2 // 图片
	MsgAudio MsgType = 3 // 音频
	MsgFile  MsgType = 4 // 文件
)

// SendType 发送类型
type SendType int

const (
	SendPrivate   SendType = 1 // 私聊
	SendGroup     SendType = 2 // 群聊
	SendBroadcast SendType = 3 // 广播
)

// MsgStatus 消息状态
type MsgStatus int

const (
	MsgUnsent    MsgStatus = 1 // 未发送
	MsgSent      MsgStatus = 2 // 已发送
	MsgDelivered MsgStatus = 3 // 已送达
	MsgRead      MsgStatus = 4 // 已读
)

// Message 消息主表
type Message struct {
	gorm.Model

	MessageID string `gorm:"uniqueIndex;size:64"`
	// ===== 发送 / 接收方 =====
	SenderID   uint64 `gorm:"index"` // 发送者
	ReceiverID uint64 `gorm:"index"` // 接收者（群聊时为 group_id）

	// ===== 消息分类 =====
	SendType SendType `gorm:"type:tinyint;comment:发送类型 1私聊 2群聊 3广播"`
	MsgType  MsgType  `gorm:"type:tinyint;comment:消息类型 1文字 2图片 3音频 4文件"`

	// ===== 消息内容 =====
	Content string  `gorm:"type:text"`         // 文本消息
	URL     string  `gorm:"type:varchar(512)"` // 图片 / 音频 / 文件地址
	FileID  *uint64 // (文件表索引id)可选

	// ===== 消息状态 =====
	Status MsgStatus `gorm:"type:tinyint;default:1"` // 1未发送 2已发送 3已送达 4已读

	// ===== 时间 =====
	ReadTime *time.Time
}

// TableName 表名
func (table *Message) TableName() string {
	return "message"
}
