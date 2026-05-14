package model

import (
	"gorm.io/gorm"
	"time"
)

type UserBasic struct {
	gorm.Model
	//名称（唯一索引）
	Name string `gorm:"type:varchar(50);uniqueIndex:uk_user_name"`
	//密码
	Password string
	//唯一身份标识
	Identity string
	//手机号码
	Phone string
	//电子邮箱
	Email string
	//客户端ip
	ClientIp string
	//客户端端口
	ClientPort string
	//登录时间
	LoginTime *time.Time
	//心跳时间
	HeartbeatTime *time.Time
	//登出时间
	LogoutTime *time.Time
	//是否登出
	IsLogout bool
	//设备信息
	DeviceInfo string
}

func (table *UserBasic) TableName() string {
	return "user_basic"
}
