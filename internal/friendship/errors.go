package friendship

import "errors"

var (
	// ErrInvalidInput 表示好友参数不符合业务规则。
	ErrInvalidInput = errors.New("好友参数不合法")

	// ErrUserNotFound 表示目标用户不存在或已禁用。
	ErrUserNotFound = errors.New("用户不存在")

	// ErrAlreadyFriends 表示双方已经是好友。
	ErrAlreadyFriends = errors.New("双方已经是好友")

	// ErrRequestPending 表示双方已有待处理申请。
	ErrRequestPending = errors.New("双方已有待处理的好友申请")

	// ErrRequestNotFound 表示好友申请不存在。
	ErrRequestNotFound = errors.New("好友申请不存在")

	// ErrRequestForbidden 表示当前用户无权处理该申请。
	ErrRequestForbidden = errors.New("无权处理该好友申请")

	// ErrFriendshipNotFound 表示好友关系不存在。
	ErrFriendshipNotFound = errors.New("好友关系不存在")

	// ErrUserBlocked 表示双方存在拉黑关系，不能继续社交互动。
	ErrUserBlocked = errors.New("双方存在拉黑关系")
)
