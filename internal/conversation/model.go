package conversation

import "time"

const (
	conversationTypeDirect      = 1
	conversationTypeGroup       = 2
	conversationStatusOpen      = 1
	conversationStatusDissolved = 3
	groupMemberRole             = 1
	groupAdminRole              = 2
	groupOwnerRole              = 3
	invitationStatusPending     = 1
	invitationStatusAccepted    = 2
	invitationStatusRejected    = 3
	invitationStatusExpired     = 4
	invitationStatusCancelled   = 5
)

type conversation struct {
	// ID 是会话 Snowflake ID。
	ID uint64 `gorm:"column:id;primaryKey;autoIncrement:false"`

	// ConversationType 表示私聊或群聊。
	ConversationType int `gorm:"column:conversation_type"`

	// DirectLowUserID 是私聊用户对中较小的 Snowflake ID。
	DirectLowUserID *uint64 `gorm:"column:direct_low_user_id"`

	// DirectHighUserID 是私聊用户对中较大的 Snowflake ID。
	DirectHighUserID *uint64 `gorm:"column:direct_high_user_id"`

	// OwnerID 是群主 Snowflake ID，私聊时为空。
	OwnerID *uint64 `gorm:"column:owner_id"`

	// Name 是群名称，私聊时为空。
	Name *string `gorm:"column:name"`

	// Title 是对当前用户可直接展示的会话标题。
	Title string `gorm:"-"`

	// AvatarURL 是群头像地址。
	AvatarURL *string `gorm:"column:avatar_url"`

	// Status 表示会话状态，1 为正常。
	Status int `gorm:"column:status"`

	// LastSeq 是会话当前最后消息序号。
	LastSeq uint64 `gorm:"column:last_seq"`

	// LastMessageID 是最近消息 Snowflake ID。
	LastMessageID *uint64 `gorm:"column:last_message_id"`

	// CreatedAt 是会话创建的 UTC 时间。
	CreatedAt time.Time `gorm:"column:created_at"`

	// UpdatedAt 是会话最近活动的 UTC 时间。
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

func (conversation) TableName() string {
	return "conversations"
}

type conversationMember struct {
	// ConversationID 是所属会话 Snowflake ID。
	ConversationID uint64 `gorm:"column:conversation_id;primaryKey"`

	// UserID 是成员用户 Snowflake ID。
	UserID uint64 `gorm:"column:user_id;primaryKey"`

	// Role 表示成员、管理员或群主。
	Role int `gorm:"column:role"`

	// LastReadSeq 是用户已读到的会话消息序号。
	LastReadSeq uint64 `gorm:"column:last_read_seq"`

	// JoinedAt 是首次加入会话的 UTC 时间。
	JoinedAt time.Time `gorm:"column:joined_at"`

	// LeftAt 是退出时间，空表示当前有效成员。
	LeftAt *time.Time `gorm:"column:left_at"`

	// UpdatedAt 是成员记录最近修改的 UTC 时间。
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

func (conversationMember) TableName() string {
	return "conversation_members"
}

type groupInvitation struct {
	// ID 是群邀请 Snowflake ID。
	ID uint64 `gorm:"column:id;primaryKey;autoIncrement:false"`

	// ConversationID 是目标群会话 Snowflake ID。
	ConversationID uint64 `gorm:"column:conversation_id"`

	// InviterID 是邀请人 Snowflake ID。
	InviterID uint64 `gorm:"column:inviter_id"`

	// InviteRemark 是邀请人填写的入群理由。
	InviteRemark string `gorm:"column:invite_remark"`

	// InviteeID 是被邀请人 Snowflake ID。
	InviteeID uint64 `gorm:"column:invitee_id"`

	// ResponseRemark 是被邀请人同意或拒绝时填写的回复。
	ResponseRemark *string `gorm:"column:response_remark"`

	// Status 表示邀请状态，1 待处理、2 已接受、3 已拒绝、4 已过期、5 已取消。
	Status int `gorm:"column:status"`

	// CreatedAt 是本次邀请创建的 UTC 时间。
	CreatedAt time.Time `gorm:"column:created_at"`

	// ExpiresAt 是邀请失效的 UTC 时间。
	ExpiresAt time.Time `gorm:"column:expires_at"`

	// RespondedAt 是接受、拒绝或取消的 UTC 时间。
	RespondedAt *time.Time `gorm:"column:responded_at"`
}

func (groupInvitation) TableName() string {
	return "group_invitations"
}

type receivedInvitationRecord struct {
	InvitationID     uint64    `gorm:"column:invitation_id"`
	ConversationID   uint64    `gorm:"column:conversation_id"`
	ConversationName string    `gorm:"column:conversation_name"`
	AvatarURL        *string   `gorm:"column:avatar_url"`
	InviterID        uint64    `gorm:"column:inviter_id"`
	InviterName      string    `gorm:"column:inviter_name"`
	InviteRemark     string    `gorm:"column:invite_remark"`
	ExpiresAt        time.Time `gorm:"column:expires_at"`
	CreatedAt        time.Time `gorm:"column:created_at"`
}

type groupInvitationRecord struct {
	InvitationID   uint64     `gorm:"column:invitation_id"`
	InviteeID      uint64     `gorm:"column:invitee_id"`
	Username       string     `gorm:"column:username"`
	Nickname       string     `gorm:"column:nickname"`
	AvatarURL      *string    `gorm:"column:avatar_url"`
	InviterID      uint64     `gorm:"column:inviter_id"`
	InviteRemark   string     `gorm:"column:invite_remark"`
	Status         int        `gorm:"column:status"`
	ResponseRemark *string    `gorm:"column:response_remark"`
	ExpiresAt      time.Time  `gorm:"column:expires_at"`
	CreatedAt      time.Time  `gorm:"column:created_at"`
	RespondedAt    *time.Time `gorm:"column:responded_at"`
}

type groupInvitationEventPayload struct {
	// InvitationID 是群邀请 Snowflake ID 字符串。
	InvitationID string `json:"invitation_id"`

	// ConversationID 是目标群会话 Snowflake ID 字符串。
	ConversationID string `json:"conversation_id"`

	// InviteeID 是被邀请用户 Snowflake ID 字符串。
	InviteeID string `json:"invitee_id"`
}

type conversationSummary struct {
	// ID 是会话 Snowflake ID。
	ID uint64 `gorm:"column:id"`

	// ConversationType 表示私聊或群聊。
	ConversationType int `gorm:"column:conversation_type"`

	// Name 是群名称，私聊时为空。
	Name *string `gorm:"column:name"`

	// Title 是对当前用户可直接展示的会话标题。
	Title string `gorm:"column:title"`

	// PeerUserID 是私聊对方 Snowflake ID，群聊时为空。
	PeerUserID *uint64 `gorm:"column:peer_user_id"`

	// AvatarURL 是群头像。
	AvatarURL *string `gorm:"column:avatar_url"`

	// LastSeq 是会话最后消息序号。
	LastSeq uint64 `gorm:"column:last_seq"`

	// LastReadSeq 是当前用户最后已读序号。
	LastReadSeq uint64 `gorm:"column:last_read_seq"`

	// MemberCount 是当前有效成员数量。
	MemberCount uint64 `gorm:"column:member_count"`

	// UpdatedAt 是会话最近活动时间。
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

type memberRecord struct {
	// UserID 是成员用户 Snowflake ID。
	UserID uint64 `gorm:"column:user_id"`

	// Username 是成员登录名。
	Username string `gorm:"column:username"`

	// Nickname 是成员昵称。
	Nickname string `gorm:"column:nickname"`

	// AvatarURL 是成员头像地址。
	AvatarURL string `gorm:"column:avatar_url"`

	// Role 表示成员角色，1 成员、2 管理员、3 群主。
	Role int `gorm:"column:role"`

	// JoinedAt 是成员加入会话的 UTC 时间。
	JoinedAt time.Time `gorm:"column:joined_at"`
}
