package identity

import "time"

const userStatusActive = 1

type user struct {
	// ID 是用户 Snowflake ID，数据库类型为 BIGINT UNSIGNED。
	ID uint64 `gorm:"column:id;primaryKey;autoIncrement:false"`

	// Username 是唯一登录名，大小写由数据库排序规则统一处理。
	Username string `gorm:"column:username"`

	// PasswordHash 是 bcrypt 哈希，禁止保存或返回明文密码。
	PasswordHash string `gorm:"column:password_hash"`

	// Status 表示账号状态，1 为正常，2 为禁用。
	Status int `gorm:"column:status"`

	// CreatedAt 是用户注册的 UTC 时间。
	CreatedAt time.Time `gorm:"column:created_at"`

	// UpdatedAt 是用户记录最近修改的 UTC 时间。
	UpdatedAt time.Time `gorm:"column:updated_at"`

	// DeletedAt 是软删除时间，为空表示账号仍保留。
	DeletedAt *time.Time `gorm:"column:deleted_at"`
}

func (user) TableName() string {
	return "users"
}

type userProfile struct {
	// UserID 与 users.id 相同，同时作为一对一资料表主键。
	UserID uint64 `gorm:"column:user_id;primaryKey"`

	// Nickname 是用户对外展示的昵称。
	Nickname string `gorm:"column:nickname"`

	// AvatarURL 是用户头像地址，当前注册流程暂时为空。
	AvatarURL *string `gorm:"column:avatar_url"`

	// Bio 是个人简介，当前注册流程暂时为空。
	Bio *string `gorm:"column:bio"`

	// CreatedAt 是资料创建的 UTC 时间。
	CreatedAt time.Time `gorm:"column:created_at"`

	// UpdatedAt 是资料最近修改的 UTC 时间。
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

func (userProfile) TableName() string {
	return "user_profiles"
}

type device struct {
	// ID 是设备 Snowflake ID。
	ID uint64 `gorm:"column:id;primaryKey;autoIncrement:false"`

	// UserID 是设备所属用户 Snowflake ID。
	UserID uint64 `gorm:"column:user_id"`

	// Platform 表示设备平台：1 Web、2 Android、3 iOS、4 Desktop。
	Platform int `gorm:"column:platform"`

	// DeviceName 是客户端展示名称。
	DeviceName string `gorm:"column:device_name"`

	// LastSeenAt 是设备最近一次申请 WebSocket Ticket 的 UTC 时间。
	LastSeenAt *time.Time `gorm:"column:last_seen_at"`

	// RevokedAt 非空表示设备会话已撤销。
	RevokedAt *time.Time `gorm:"column:revoked_at"`

	// CreatedAt 是设备首次注册的 UTC 时间。
	CreatedAt time.Time `gorm:"column:created_at"`
}

func (device) TableName() string {
	return "devices"
}

type refreshToken struct {
	// ID 是 Refresh Token Snowflake ID。
	ID uint64 `gorm:"column:id;primaryKey;autoIncrement:false"`

	// UserID 是 Refresh Token 所属用户 Snowflake ID。
	UserID uint64 `gorm:"column:user_id"`

	// DeviceID 是 Refresh Token 绑定的设备 Snowflake ID。
	DeviceID uint64 `gorm:"column:device_id"`

	// TokenHash 是原始 Refresh Token 的 SHA-256 十六进制摘要。
	TokenHash string `gorm:"column:token_hash"`

	// ExpiresAt 是 Refresh Token 的 UTC 过期时间。
	ExpiresAt time.Time `gorm:"column:expires_at"`

	// RevokedAt 是撤销时间，空表示仍可使用。
	RevokedAt *time.Time `gorm:"column:revoked_at"`

	// CreatedAt 是 Refresh Token 创建的 UTC 时间。
	CreatedAt time.Time `gorm:"column:created_at"`
}

func (refreshToken) TableName() string {
	return "refresh_tokens"
}
