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
