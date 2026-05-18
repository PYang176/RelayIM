package utils

import (
	"fmt"
	"ginChatProject/internal/config"
	"ginChatProject/utils/jwt"
	"ginChatProject/utils/redis_util"
	"ginChatProject/utils/snowflake"
	"github.com/spf13/viper"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"log"
	"os"
	"time"
)

func InitConfig() {
	viper.SetConfigName("app")
	viper.AddConfigPath("./internal/config")
	err := viper.ReadInConfig()
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println("config app:", viper.Get("app"))
	fmt.Println("config mysql:", viper.Get("mysql"))
}

// InitMySQL 初始化Mysql数据库连接对象
func InitMySQL(cfg config.MySQLConfig) (*gorm.DB, error) {
	//读取yml配置，封装进结构体中
	if err := viper.UnmarshalKey("mysql", &cfg); err != nil {
		panic(fmt.Sprintf("解析 mysql 配置失败: %v", err))
	}
	fmt.Println(cfg)

	//将结构体的数据拼接成dsn
	dsn := GetMySQLDsn(cfg)
	fmt.Println("我是dsn---------------------", dsn)

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
	fmt.Println("数据库连接成功")

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
func InitJWT() {
	err := viper.UnmarshalKey("jwt", &jwt.JwtCfg)
	if err != nil {
		panic(fmt.Sprintf("解析 jwt 配置失败: %v", err))
	}
}

// InitRedis 从配置文件初始化redis连接
func InitRedis() {
	var redisCfg config.RedisConfig
	err := viper.UnmarshalKey("redis", &redisCfg)
	if err != nil {
		panic(fmt.Sprintf("解析 redis 配置失败: %v", err))
	}
	redis_util.InitRedis(redisCfg)
}

// InitSnowFlake 从配置文件初始化雪花算法
func InitSnowFlake() {
	workID := viper.GetInt64("snowflake.workID")
	fmt.Println("workID:", workID)
	if err := snowflake.Init(workID); err != nil {
		log.Fatal(err)
	}
}
