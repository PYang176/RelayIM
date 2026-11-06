package bootstrap

import (
	"fmt"
	"ginChatProject/internal/config"
	"ginChatProject/internal/platform/jwt"
	"ginChatProject/internal/platform/redisstore"
	"ginChatProject/internal/platform/snowflake"
	"github.com/spf13/viper"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"log"
	"os"
	"time"
)

func InitConfig() error {
	viper.SetConfigName("app")
	viper.AddConfigPath("./internal/config")
	if err := viper.ReadInConfig(); err != nil {
		return fmt.Errorf("读取应用配置失败: %w", err)
	}
	return nil
}

// InitMySQL 初始化Mysql数据库连接对象
func InitMySQL(cfg config.MySQLConfig) (*gorm.DB, error) {
	dsn := GetMySQLDsn(cfg)

	// 自定义日志模版，打印SQL语句
	newLogger := logger.New(
		log.New(os.Stdout, "\r\n", log.LstdFlags),
		logger.Config{
			SlowThreshold: time.Second, // 慢 SQL 阈值
			LogLevel:      logger.Info, // 打印所有 SQL
			Colorful:      true,        // 彩色输出
		},
	)

	//获取数据库连接对象
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{Logger: newLogger})
	if err != nil {
		return nil, err
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}

	sqlDB.SetMaxOpenConns(200)          // 最大连接数（至少是压测并发数）
	sqlDB.SetMaxIdleConns(50)           // 空闲连接数
	sqlDB.SetConnMaxLifetime(time.Hour) // 连接复用时间
	return db, nil
}

// InitJWT 从配置文件初始化密钥和过期时间
func InitJWT() error {
	if err := viper.UnmarshalKey("jwt", &jwt.JwtCfg); err != nil {
		return fmt.Errorf("解析 JWT 配置失败: %w", err)
	}
	return nil
}

// InitRedis 从配置文件初始化redis连接
func InitRedis() error {
	var redisCfg config.RedisConfig
	if err := viper.UnmarshalKey("redis", &redisCfg); err != nil {
		return fmt.Errorf("解析 Redis 配置失败: %w", err)
	}
	if err := redisstore.InitRedis(redisCfg); err != nil {
		return fmt.Errorf("初始化 Redis 失败: %w", err)
	}
	return nil
}

// InitSnowFlake 从配置文件初始化雪花算法
func InitSnowFlake() error {
	workID := viper.GetInt64("snowflake.workID")
	if err := snowflake.Init(workID); err != nil {
		return fmt.Errorf("初始化 Snowflake 失败: %w", err)
	}
	return nil
}
