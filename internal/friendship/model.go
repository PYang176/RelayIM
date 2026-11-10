package friendship

import "time"

const (
	friendshipStatusNormal  = 1
	friendshipStatusDeleted = 2

	requestStatusPending  = 1
	requestStatusAccepted = 2
	requestStatusRejected = 3
)

type friendship struct {
	// ID 是好友关系 Snowflake ID。
	ID uint64 `gorm:"column:id;primaryKey;autoIncrement:false"`

	// UserLowID 是用户对中较小的 Snowflake 用户 ID。
	UserLowID uint64 `gorm:"column:user_low_id"`

	// UserHighID 是用户对中较大的 Snowflake 用户 ID。
	UserHighID uint64 `gorm:"column:user_high_id"`

	// Status 表示关系状态，1 正常、2 已删除。
	Status int `gorm:"column:status"`

	// CreatedAt 是关系创建的 UTC 时间。
	CreatedAt time.Time `gorm:"column:created_at"`

	// UpdatedAt 是关系最近修改的 UTC 时间。
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

// TableName 返回 friendship 模型对应的数据库表名
func (friendship) TableName() string {
	return "friendships"
}

type friendRequest struct {
	// ID 是好友申请 Snowflake ID。
	ID uint64 `gorm:"column:id;primaryKey;autoIncrement:false"`

	// UserLowID 是用户对中较小的 Snowflake 用户 ID。
	UserLowID uint64 `gorm:"column:user_low_id"`

	// UserHighID 是用户对中较大的 Snowflake 用户 ID。
	UserHighID uint64 `gorm:"column:user_high_id"`

	// RequesterID 是发起申请的用户 Snowflake ID。
	RequesterID uint64 `gorm:"column:requester_id"`

	// Status 表示申请状态，1 待处理、2 已同意、3 已拒绝、4 已撤回。
	Status int `gorm:"column:status"`

	// Remark 是申请附言。
	Remark *string `gorm:"column:remark"`

	// CreatedAt 是申请创建的 UTC 时间。
	CreatedAt time.Time `gorm:"column:created_at"`

	// UpdatedAt 是申请最近修改的 UTC 时间。
	UpdatedAt time.Time `gorm:"column:updated_at"`

	// RespondedAt 是接受或拒绝申请的 UTC 时间。
	RespondedAt *time.Time `gorm:"column:responded_at"`
}

// TableName 返回 friendRequest 模型对应的数据库表名
func (friendRequest) TableName() string {
	return "friend_requests"
}

type friendRemark struct {
	// FriendshipID 是好友关系 Snowflake ID。
	FriendshipID uint64 `gorm:"column:friendship_id;primaryKey"`

	// UserID 是设置备注的用户 Snowflake ID。
	UserID uint64 `gorm:"column:user_id;primaryKey"`

	// Remark 是该用户看到的独立好友备注。
	Remark string `gorm:"column:remark"`

	// CreatedAt 是备注创建的 UTC 时间。
	CreatedAt time.Time `gorm:"column:created_at"`

	// UpdatedAt 是备注最近修改的 UTC 时间。
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

// TableName 返回 friendRemark 模型对应的数据库表名
func (friendRemark) TableName() string {
	return "friend_remarks"
}

type userBlock struct {
	// BlockerID 是执行拉黑的用户 Snowflake ID。
	BlockerID uint64 `gorm:"column:blocker_id;primaryKey"`

	// BlockedID 是被拉黑的用户 Snowflake ID。
	BlockedID uint64 `gorm:"column:blocked_id;primaryKey"`

	// CreatedAt 是拉黑关系创建的 UTC 时间。
	CreatedAt time.Time `gorm:"column:created_at"`
}

// TableName 返回 userBlock 模型对应的数据库表名
func (userBlock) TableName() string {
	return "user_blocks"
}

type userSearchRecord struct {
	ID        uint64  `gorm:"column:id"`
	Username  string  `gorm:"column:username"`
	Nickname  string  `gorm:"column:nickname"`
	AvatarURL *string `gorm:"column:avatar_url"`
}

type friendRecord struct {
	FriendshipID uint64  `gorm:"column:friendship_id"`
	FriendUserID uint64  `gorm:"column:friend_user_id"`
	Username     string  `gorm:"column:username"`
	Nickname     string  `gorm:"column:nickname"`
	AvatarURL    *string `gorm:"column:avatar_url"`
	Remark       string  `gorm:"column:remark"`
	BlockedByMe  bool    `gorm:"column:blocked_by_me"`
}

type blockedUserRecord struct {
	UserID    uint64    `gorm:"column:user_id"`
	Username  string    `gorm:"column:username"`
	Nickname  string    `gorm:"column:nickname"`
	AvatarURL *string   `gorm:"column:avatar_url"`
	CreatedAt time.Time `gorm:"column:created_at"`
}

type requestRecord struct {
	ID          uint64    `gorm:"column:id"`
	RequesterID uint64    `gorm:"column:requester_id"`
	UserID      uint64    `gorm:"column:user_id"`
	Username    string    `gorm:"column:username"`
	Nickname    string    `gorm:"column:nickname"`
	AvatarURL   *string   `gorm:"column:avatar_url"`
	Status      int       `gorm:"column:status"`
	Remark      *string   `gorm:"column:remark"`
	CreatedAt   time.Time `gorm:"column:created_at"`
}
