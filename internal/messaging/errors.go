package messaging

import "errors"

var (
	// ErrInvalidMessage 表示消息参数不合法。
	ErrInvalidMessage = errors.New("消息参数不合法")
	// ErrInvalidSearchQuery 表示消息搜索词长度不符合规则。
	ErrInvalidSearchQuery = errors.New("搜索词需为 1 到 64 个字符")
	// ErrConversationNotFound 表示会话不存在。
	ErrConversationNotFound = errors.New("会话不存在")
	// ErrConversationClosed 表示会话不可发送消息。
	ErrConversationClosed = errors.New("会话已关闭")
	// ErrNotMember 表示发送者不是会话成员。
	ErrNotMember = errors.New("发送者不是会话成员")
	// ErrUserBlocked 表示私聊双方存在拉黑关系。
	ErrUserBlocked = errors.New("双方存在拉黑关系")
	// ErrNotFriends 表示私聊双方已不是有效好友关系。
	ErrNotFriends = errors.New("你不是对方的好友")
	// ErrInvalidReply 表示引用消息不属于当前会话。
	ErrInvalidReply = errors.New("引用的消息无效")
	// ErrClientMessageConflict 表示同一客户端消息 ID 被用于其他会话。
	ErrClientMessageConflict = errors.New("客户端消息 ID 已被其他会话使用")

	// ErrOutboxLeaseLost 表示发布实例已失去事件处理租约。
	ErrOutboxLeaseLost = errors.New("Outbox 事件租约已失效")

	// ErrInvalidEvent 表示 RabbitMQ 事件信封或业务载荷不合法。
	ErrInvalidEvent = errors.New("消息事件不合法")

	// ErrMessageEventNotFound 表示事件引用的消息事实不存在。
	ErrMessageEventNotFound = errors.New("消息事件对应的消息不存在")

	// ErrMessageNotFound 表示指定消息不存在或不属于当前会话。
	ErrMessageNotFound = errors.New("消息不存在")

	// ErrRecallForbidden 表示当前用户无权撤回指定消息。
	ErrRecallForbidden = errors.New("没有权限撤回该消息")

	// ErrRecallExpired 表示消息已超过允许撤回的时间窗口。
	ErrRecallExpired = errors.New("消息已超过可撤回时间")

	// ErrDeviceUnavailable 表示设备不存在、已撤销或不属于当前用户。
	ErrDeviceUnavailable = errors.New("设备不存在或已失效")

	// ErrInvalidReadSequence 表示已读序号超过会话当前最大序号。
	ErrInvalidReadSequence = errors.New("已读序号超过会话当前消息序号")

	// ErrInvalidAttachment 表示附件元数据或上传内容不合法。
	ErrInvalidAttachment = errors.New("附件不合法")

	// ErrAttachmentNotFound 表示附件不存在或当前用户无权访问。
	ErrAttachmentNotFound = errors.New("附件不存在或无权访问")

	// ErrAttachmentStorageUnavailable 表示对象存储暂时不可用。
	ErrAttachmentStorageUnavailable = errors.New("附件存储暂不可用")

	// ErrAttachmentAlreadyUsed 表示附件已绑定到其他消息。
	ErrAttachmentAlreadyUsed = errors.New("附件已绑定其他消息")
)
