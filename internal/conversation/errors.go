package conversation

import "errors"

var (
	// ErrInvalidInput 表示会话参数不符合业务规则。
	ErrInvalidInput = errors.New("会话参数不合法")

	// ErrSameUser 表示用户尝试与自己创建私聊。
	ErrSameUser = errors.New("不能与自己创建私聊")

	// ErrUserNotFound 表示会话成员对应的用户不存在。
	ErrUserNotFound = errors.New("会话用户不存在")

	// ErrConversationNotFound 表示当前用户无法访问该会话。
	ErrConversationNotFound = errors.New("会话不存在或无权访问")
)
