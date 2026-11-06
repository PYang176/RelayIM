package messaging

import "errors"

var (
	// ErrInvalidMessage 表示消息参数不合法。
	ErrInvalidMessage = errors.New("消息参数不合法")
	// ErrConversationNotFound 表示会话不存在。
	ErrConversationNotFound = errors.New("会话不存在")
	// ErrConversationClosed 表示会话不可发送消息。
	ErrConversationClosed = errors.New("会话已关闭")
	// ErrNotMember 表示发送者不是会话成员。
	ErrNotMember = errors.New("发送者不是会话成员")
	// ErrInvalidReply 表示引用消息不属于当前会话。
	ErrInvalidReply = errors.New("引用的消息无效")
	// ErrClientMessageConflict 表示同一客户端消息 ID 被用于其他会话。
	ErrClientMessageConflict = errors.New("客户端消息 ID 已被其他会话使用")
)
