package conversation

import "errors"

var (
	// ErrInvalidInput 表示会话参数不符合业务规则。
	ErrInvalidInput = errors.New("会话参数不合法")

	// ErrSameUser 表示用户尝试与自己创建私聊。
	ErrSameUser = errors.New("不能与自己创建私聊")

	// ErrUserNotFound 表示会话成员对应的用户不存在。
	ErrUserNotFound = errors.New("会话用户不存在")

	// ErrNotFriend 表示创建群聊时选择了非好友用户。
	ErrNotFriend = errors.New("只能邀请好友加入群聊")

	// ErrNotGroup 表示目标会话不是群聊。
	ErrNotGroup = errors.New("目标会话不是群聊")

	// ErrPermissionDenied 表示当前成员没有执行该群管理操作的权限。
	ErrPermissionDenied = errors.New("没有权限执行该群管理操作")

	// ErrMemberNotFound 表示目标用户不是群聊有效成员。
	ErrMemberNotFound = errors.New("群成员不存在")

	// ErrGroupFull 表示群成员数量已经达到上限。
	ErrGroupFull = errors.New("群成员数量已达到上限")

	// ErrOwnerCannotLeave 表示群主在转让或解散群之前不能退出群聊。
	ErrOwnerCannotLeave = errors.New("群主不能直接退出群聊")

	// ErrConversationNotFound 表示当前用户无法访问该会话。
	ErrConversationNotFound = errors.New("会话不存在或无权访问")
)
