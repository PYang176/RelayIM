package model

import (
	"gorm.io/gorm"
	"time"
)

type UserBasic struct {
	gorm.Model
	// Name 是登录用户名，数据库唯一索引防止重复注册。
	Name string `gorm:"type:varchar(50);uniqueIndex:uk_user_name"`
	// Password 保存密码哈希，禁止保存或输出明文密码。
	Password string
	// Identity 是用户的业务身份标识，用于扩展区分外部身份来源。
	Identity string
	// Phone 是用户手机号，仅用于资料和后续登录扩展。
	Phone string
	// Email 是用户邮箱，仅用于资料和通知扩展。
	Email string
	// ClientIp 是最近一次登录客户端 IP，不能直接暴露给普通用户。
	ClientIp string
	// ClientPort 是最近一次登录客户端端口，仅用于连接排查。
	ClientPort string
	// LoginTime 是最近一次登录时间。
	LoginTime *time.Time
	// HeartbeatTime 是最近同步到数据库的心跳时间，实时状态以 Redis 为准。
	HeartbeatTime *time.Time
	// LogoutTime 是最近一次登出时间。
	LogoutTime *time.Time
	// IsLogout 表示用户当前是否处于登出状态。
	IsLogout bool
	// DeviceInfo 是最近登录客户端上报的设备描述。
	DeviceInfo string
}

func (table *UserBasic) TableName() string {
	return "user_basic"
}
