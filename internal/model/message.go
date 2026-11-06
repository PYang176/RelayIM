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
	MsgUnsent      MsgStatus = 1 // 未发送
	MsgSent        MsgStatus = 2 // 已发送
	MsgUndelivered MsgStatus = 3 // 未送达 （离线未推送）
	MsgDelivered   MsgStatus = 4 // 已送达（在线并推送成功）
	MsgRead        MsgStatus = 5 // 已读
)

// Message 消息主表
type Message struct {
	gorm.Model

	// MessageID 是旧消息表的业务消息 ID，当前原型用 Snowflake 字符串生成。
	MessageID string `gorm:"uniqueIndex;size:64"`
	// ===== 发送 / 接收方 =====
	// SenderID 是发送者用户 ID。
	SenderID uint64 `gorm:"index"`
	// ReceiverID 是私聊接收者或群聊群 ID，新会话模型不再使用该字段。
	ReceiverID uint64 `gorm:"index"`

	// ===== 消息分类 =====
	// SendType 表示私聊、群聊或广播。
	SendType SendType `gorm:"type:tinyint;comment:发送类型 1私聊 2群聊 3广播"`
	// MsgType 表示文本、图片、音频或文件消息。
	MsgType MsgType `gorm:"type:tinyint;comment:消息类型 1文字 2图片 3音频 4文件"`

	// ===== 消息内容 =====
	// Content 是文本消息正文。
	Content string `gorm:"type:text"`
	// URL 是旧媒体消息的图片、音频或文件地址。
	URL string `gorm:"type:varchar(512)"`
	// FileID 是旧文件表索引 ID，新附件模型使用 attachment_id。
	FileID *uint64

	// ===== 消息状态 =====
	// Status 是旧全局消息状态；新模型把送达和已读拆分，不再复用该字段。
	Status MsgStatus `gorm:"type:tinyint;default:1"`

	// ===== 时间 =====
	// ReadTime 是旧模型记录的消息已读时间。
	ReadTime *time.Time
}

// TableName 表名
func (table *Message) TableName() string {
	return "message"
}
