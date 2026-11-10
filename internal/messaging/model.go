package messaging

import "time"

const (
	conversationStatusActive = 1
	conversationTypeDirect   = 1
	conversationTypeGroup    = 2
	groupRoleAdmin           = 2
	groupRoleOwner           = 3
	messageTypeText          = 1
	messageTypeImage         = 2
	messageTypeFile          = 3
	outboxStatusPending      = 0
	outboxStatusProcessing   = 1
	outboxStatusPublished    = 2
	outboxStatusDead         = 3
	attachmentStatusPending  = 1
	attachmentStatusReady    = 2
	attachmentStatusBlocked  = 3
	attachmentStatusDeleted  = 4
)

type conversation struct {
	// ID 是会话 Snowflake ID，也是会话内消息的外键目标。
	ID uint64 `gorm:"column:id;primaryKey"`

	// ConversationType 表示私聊或群聊，2 为群聊。
	ConversationType int `gorm:"column:conversation_type"`

	// Status 表示会话状态，1 为正常，非 1 时当前发送流程会拒绝写入。
	Status int `gorm:"column:status"`

	// DirectLowUserID 是私聊用户对中较小的 Snowflake 用户 ID。
	DirectLowUserID *uint64 `gorm:"column:direct_low_user_id"`

	// DirectHighUserID 是私聊用户对中较大的 Snowflake 用户 ID。
	DirectHighUserID *uint64 `gorm:"column:direct_high_user_id"`

	// LastSeq 是会话已分配的最大消息序号，由发送事务持有行锁后递增。
	LastSeq uint64 `gorm:"column:last_seq"`

	// LastMessageID 是最近一条消息的 Snowflake ID，用于会话摘要展示。
	LastMessageID *uint64 `gorm:"column:last_message_id"`
}

func (conversation) TableName() string {
	return "conversations"
}

type conversationMember struct {
	// ConversationID 是成员所属会话的 Snowflake ID。
	ConversationID uint64 `gorm:"column:conversation_id"`

	// UserID 是成员用户的 Snowflake ID。
	UserID uint64 `gorm:"column:user_id"`

	// Role 表示群内角色，1 成员、2 管理员、3 群主。
	Role int `gorm:"column:role"`

	// LastReadSeq 是成员最后已读的会话消息序号。
	LastReadSeq uint64 `gorm:"column:last_read_seq"`

	// LeftAt 是退群时间，为空表示当前仍是有效成员。
	LeftAt *time.Time `gorm:"column:left_at"`
}

func (conversationMember) TableName() string {
	return "conversation_members"
}

type device struct {
	// ID 是设备 Snowflake ID。
	ID uint64 `gorm:"column:id;primaryKey"`

	// UserID 是设备所属用户 Snowflake ID。
	UserID uint64 `gorm:"column:user_id"`

	// RevokedAt 非空表示设备已撤销。
	RevokedAt *time.Time `gorm:"column:revoked_at"`
}

func (device) TableName() string {
	return "devices"
}

type message struct {
	// ID 是消息 Snowflake ID，由应用生成而不是数据库自增。
	ID uint64 `gorm:"column:id;primaryKey;autoIncrement:false"`

	// ConversationID 是消息所属会话的 Snowflake ID。
	ConversationID uint64 `gorm:"column:conversation_id"`

	// Seq 是会话内严格递增的消息序号，用于排序、补拉和未读计算。
	Seq uint64 `gorm:"column:seq"`

	// SenderID 是发送者用户的 Snowflake ID。
	SenderID uint64 `gorm:"column:sender_id"`

	// ClientMessageID 是客户端生成的幂等 ID，同一发送者内唯一。
	ClientMessageID string `gorm:"column:client_msg_id"`

	// MessageType 表示消息类型，1 为文本；其他类型由后续媒体链路支持。
	MessageType int `gorm:"column:message_type"`

	// Content 是文本正文；当前事务入口只创建文本消息。
	Content string `gorm:"column:content"`

	// ReplyToMessageID 是被引用消息的 Snowflake ID，必须属于同一会话。
	ReplyToMessageID *uint64 `gorm:"column:reply_to_message_id"`

	// AttachmentID 是图片或文件消息关联的附件 Snowflake ID。
	AttachmentID *uint64 `gorm:"column:attachment_id"`

	// AttachmentName 是查询历史消息时联表得到的附件文件名。
	AttachmentName *string `gorm:"->;column:attachment_name"`

	// AttachmentObjectKey 是查询历史消息时联表得到的对象键。
	AttachmentObjectKey *string `gorm:"->;column:attachment_object_key"`

	// SenderName 是消息搜索结果联表得到的发送者公开名称。
	SenderName *string `gorm:"->;column:sender_name"`

	// CreatedAt 是服务端接受消息的 UTC 时间。
	CreatedAt time.Time `gorm:"column:created_at"`

	// RecalledAt 是消息撤回的 UTC 时间，为空表示未撤回。
	RecalledAt *time.Time `gorm:"column:recalled_at"`

	// RecalledBy 是执行撤回的用户 Snowflake ID。
	RecalledBy *uint64 `gorm:"column:recalled_by"`
}

func (message) TableName() string {
	return "messages"
}

type messageDelivery struct {
	// MessageID 是已送达消息 Snowflake ID。
	MessageID uint64 `gorm:"column:message_id;primaryKey"`

	// DeviceID 是接收设备 Snowflake ID。
	DeviceID uint64 `gorm:"column:device_id;primaryKey"`

	// DeliveredAt 是设备确认送达的 UTC 时间。
	DeliveredAt time.Time `gorm:"column:delivered_at"`
}

func (messageDelivery) TableName() string {
	return "message_deliveries"
}

type attachment struct {
	// ID 是附件 Snowflake ID。
	ID uint64 `gorm:"column:id;primaryKey;autoIncrement:false"`

	// OwnerID 是上传附件的用户 Snowflake ID。
	OwnerID uint64 `gorm:"column:owner_id"`

	// ObjectKey 是对象存储内部键，不直接暴露给客户端。
	ObjectKey string `gorm:"column:object_key"`

	// OriginalName 是用户上传时的原始文件名。
	OriginalName string `gorm:"column:original_name"`

	// MIMEType 是服务端检测后的 MIME 类型。
	MIMEType string `gorm:"column:mime_type"`

	// SizeBytes 是附件字节数。
	SizeBytes int64 `gorm:"column:size_bytes"`

	// Status 表示附件状态：1 待确认、2 可用、3 阻止、4 删除。
	Status int `gorm:"column:status"`

	// CreatedAt 是附件元数据创建的 UTC 时间。
	CreatedAt time.Time `gorm:"column:created_at"`

	// FinalizedAt 是附件上传并确认完成的 UTC 时间。
	FinalizedAt *time.Time `gorm:"column:finalized_at"`
}

func (attachment) TableName() string {
	return "attachments"
}

type outboxEvent struct {
	// ID 是 Outbox 表自增扫描游标，不暴露给客户端。
	ID uint64 `gorm:"column:id;primaryKey;autoIncrement"`

	// EventID 是 RabbitMQ 事件幂等使用的 Snowflake ID。
	EventID uint64 `gorm:"column:event_id"`

	// AggregateType 是事件所属聚合类型，当前固定为 conversation。
	AggregateType string `gorm:"column:aggregate_type"`

	// AggregateID 是聚合 Snowflake ID，当前保存会话 ID。
	AggregateID uint64 `gorm:"column:aggregate_id"`

	// EventType 是版本化事件名，当前为 message.created.v1。
	EventType string `gorm:"column:event_type"`

	// Payload 是 RabbitMQ 发布的最小 JSON 载荷。
	Payload string `gorm:"column:payload"`

	// TraceParent 是创建 Outbox 时记录的 W3C Trace 上下文。
	TraceParent *string `gorm:"column:trace_parent"`

	// Status 表示发布状态：0 待发布、1 处理中、2 已发布、3 死信。
	Status int `gorm:"column:status"`

	// AttemptCount 是发布尝试次数，用于退避和死信判定。
	AttemptCount int `gorm:"column:attempt_count"`

	// AvailableAt 是允许下一次发布尝试的最早 UTC 时间。
	AvailableAt time.Time `gorm:"column:available_at"`

	// LockedBy 是当前领取事件的发布实例标识。
	LockedBy *string `gorm:"column:locked_by"`

	// LockedAt 是发布租约开始时间，用于回收崩溃实例持有的任务。
	LockedAt *time.Time `gorm:"column:locked_at"`

	// CreatedAt 是 Outbox 事件创建时间，与业务消息同事务写入。
	CreatedAt time.Time `gorm:"column:created_at"`

	// PublishedAt 是 RabbitMQ Confirm 成功后的 UTC 时间。
	PublishedAt *time.Time `gorm:"column:published_at"`

	// LastError 保存最近一次发布失败原因，供重试和排查使用。
	LastError *string `gorm:"column:last_error"`
}

func (outboxEvent) TableName() string {
	return "outbox_events"
}

type processedEvent struct {
	// EventID 是 Consumer 已成功处理的 RabbitMQ 事件幂等 ID。
	EventID uint64 `gorm:"column:event_id;primaryKey"`

	// ConsumerName 是消费者名称，同一事件可以被不同消费者各处理一次。
	ConsumerName string `gorm:"column:consumer_name;primaryKey"`

	// ProcessedAt 是 Consumer 成功处理事件的 UTC 时间。
	ProcessedAt time.Time `gorm:"column:processed_at"`
}

func (processedEvent) TableName() string {
	return "processed_events"
}

type messageCreatedPayload struct {
	// MessageID 是消息 Snowflake ID 的字符串形式，避免跨语言精度丢失。
	MessageID string `json:"message_id"`

	// ConversationID 是会话 Snowflake ID 的字符串形式。
	ConversationID string `json:"conversation_id"`

	// SenderID 是发送者 Snowflake ID 的字符串形式。
	SenderID string `json:"sender_id"`

	// Seq 是消息在会话内的递增序号，用于 Consumer 和客户端排序。
	Seq uint64 `json:"seq"`

	// MessageType 表示文本、图片或文件消息。
	MessageType int `json:"message_type"`

	// AttachmentID 是可选附件 Snowflake ID 字符串。
	AttachmentID string `json:"attachment_id,omitempty"`

	// AttachmentName 是可选附件原始文件名。
	AttachmentName string `json:"attachment_name,omitempty"`
}

type messageRecalledPayload struct {
	// MessageID 是消息 Snowflake ID 的字符串形式。
	MessageID string `json:"message_id"`

	// ConversationID 是会话 Snowflake ID 的字符串形式。
	ConversationID string `json:"conversation_id"`

	// SenderID 是原消息发送者 Snowflake ID 字符串。
	SenderID string `json:"sender_id"`

	// RecalledBy 是执行撤回的用户 Snowflake ID 字符串。
	RecalledBy string `json:"recalled_by"`

	// Seq 是被撤回消息的会话内序号。
	Seq uint64 `json:"seq"`

	// RecalledAt 是撤回发生的 UTC 时间。
	RecalledAt time.Time `json:"recalled_at"`
}

type groupInvitationEventPayload struct {
	// InvitationID 是群邀请 Snowflake ID 字符串。
	InvitationID string `json:"invitation_id"`

	// ConversationID 是目标群会话 Snowflake ID 字符串。
	ConversationID string `json:"conversation_id"`

	// InviteeID 是被邀请用户 Snowflake ID 字符串。
	InviteeID string `json:"invitee_id"`
}
