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

	// ErrUserBlocked 表示双方存在拉黑关系，不能创建私聊。
	ErrUserBlocked = errors.New("双方存在拉黑关系")

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

	// ErrInvitationNotFound 表示群邀请不存在。
	ErrInvitationNotFound = errors.New("群邀请不存在")

	// ErrInvitationForbidden 表示当前用户不是邀请接收者。
	ErrInvitationForbidden = errors.New("无权处理该群邀请")

	// ErrInvitationHandled 表示群邀请已经处理。
	ErrInvitationHandled = errors.New("群邀请已处理")

	// ErrInvitationExpired 表示群邀请已经过期。
	ErrInvitationExpired = errors.New("群邀请已过期")

	// ErrInvitationUnavailable 表示邀请对应群聊已解散或不可加入。
	ErrInvitationUnavailable = errors.New("群邀请对应群聊不可加入")

	// ErrInvitationRemarkRequired 表示邀请人没有填写入群备注。
	ErrInvitationRemarkRequired = errors.New("群邀请备注不能为空")
)
