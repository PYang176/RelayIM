package messaging

import "time"

const (
	conversationStatusActive = 1
	messageTypeText          = 1
	outboxStatusPending      = 0
)

type conversation struct {
	ID            uint64  `gorm:"column:id;primaryKey"`
	Status        int     `gorm:"column:status"`
	LastSeq       uint64  `gorm:"column:last_seq"`
	LastMessageID *uint64 `gorm:"column:last_message_id"`
}

func (conversation) TableName() string {
	return "conversations"
}

type conversationMember struct {
	ConversationID uint64     `gorm:"column:conversation_id"`
	UserID         uint64     `gorm:"column:user_id"`
	LeftAt         *time.Time `gorm:"column:left_at"`
}

func (conversationMember) TableName() string {
	return "conversation_members"
}

type message struct {
	ID               uint64    `gorm:"column:id;primaryKey;autoIncrement:false"`
	ConversationID   uint64    `gorm:"column:conversation_id"`
	Seq              uint64    `gorm:"column:seq"`
	SenderID         uint64    `gorm:"column:sender_id"`
	ClientMessageID  string    `gorm:"column:client_msg_id"`
	MessageType      int       `gorm:"column:message_type"`
	Content          string    `gorm:"column:content"`
	ReplyToMessageID *uint64   `gorm:"column:reply_to_message_id"`
	CreatedAt        time.Time `gorm:"column:created_at"`
}

func (message) TableName() string {
	return "messages"
}

type outboxEvent struct {
	ID            uint64     `gorm:"column:id;primaryKey;autoIncrement"`
	EventID       uint64     `gorm:"column:event_id"`
	AggregateType string     `gorm:"column:aggregate_type"`
	AggregateID   uint64     `gorm:"column:aggregate_id"`
	EventType     string     `gorm:"column:event_type"`
	Payload       string     `gorm:"column:payload"`
	Status        int        `gorm:"column:status"`
	AttemptCount  int        `gorm:"column:attempt_count"`
	AvailableAt   time.Time  `gorm:"column:available_at"`
	LockedBy      *string    `gorm:"column:locked_by"`
	LockedAt      *time.Time `gorm:"column:locked_at"`
	CreatedAt     time.Time  `gorm:"column:created_at"`
	PublishedAt   *time.Time `gorm:"column:published_at"`
	LastError     *string    `gorm:"column:last_error"`
}

func (outboxEvent) TableName() string {
	return "outbox_events"
}

type messageCreatedPayload struct {
	MessageID      string `json:"message_id"`
	ConversationID string `json:"conversation_id"`
	SenderID       string `json:"sender_id"`
	Seq            uint64 `json:"seq"`
}
