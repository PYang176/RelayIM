package messaging

import "time"

const (
	conversationStatusActive = 1
	messageTypeText          = 1
	outboxStatusPending      = 0
)

type conversation struct {
	// ID 是会话 Snowflake ID，也是会话内消息的外键目标。
	ID uint64 `gorm:"column:id;primaryKey"`
	// Status 表示会话状态，1 为正常，非 1 时当前发送流程会拒绝写入。
	Status int `gorm:"column:status"`
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
	// LeftAt 是退群时间，为空表示当前仍是有效成员。
	LeftAt *time.Time `gorm:"column:left_at"`
}

func (conversationMember) TableName() string {
	return "conversation_members"
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
	// CreatedAt 是服务端接受消息的 UTC 时间。
	CreatedAt time.Time `gorm:"column:created_at"`
}

func (message) TableName() string {
	return "messages"
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

type messageCreatedPayload struct {
	// MessageID 是消息 Snowflake ID 的字符串形式，避免跨语言精度丢失。
	MessageID string `json:"message_id"`
	// ConversationID 是会话 Snowflake ID 的字符串形式。
	ConversationID string `json:"conversation_id"`
	// SenderID 是发送者 Snowflake ID 的字符串形式。
	SenderID string `json:"sender_id"`
	// Seq 是消息在会话内的递增序号，用于 Consumer 和客户端排序。
	Seq uint64 `json:"seq"`
}
