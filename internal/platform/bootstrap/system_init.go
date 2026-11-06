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
	"strings"
	"time"
)

func InitConfig() error {
	v := viper.GetViper()
	if err := bindConfigEnv(v); err != nil {
		return err
	}

	if configFile := strings.TrimSpace(os.Getenv("GINCHAT_CONFIG_FILE")); configFile != "" {
		v.SetConfigFile(configFile)
	} else {
		v.SetConfigName("app")
		v.AddConfigPath("./internal/config")
	}

	if err := v.ReadInConfig(); err != nil {
		return fmt.Errorf("读取应用配置失败: %w", err)
	}
	return nil
}

func bindConfigEnv(v *viper.Viper) error {
	// 前缀、键名替换和 AutomaticEnv 为后续普通字段提供统一覆盖规则；
	// 下面显式绑定关键字段，保证嵌套 YAML 键的环境变量名稳定可控。
	v.SetEnvPrefix("GINCHAT")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	bindings := map[string]string{
		"mysql.host":             "GINCHAT_MYSQL_HOST",
		"mysql.port":             "GINCHAT_MYSQL_PORT",
		"mysql.user":             "GINCHAT_MYSQL_USER",
		"mysql.password":         "GINCHAT_MYSQL_PASSWORD",
		"mysql.dbname":           "GINCHAT_MYSQL_DBNAME",
		"mysql.config.charset":   "GINCHAT_MYSQL_CHARSET",
		"mysql.config.parseTime": "GINCHAT_MYSQL_PARSE_TIME",
		"mysql.config.loc":       "GINCHAT_MYSQL_LOCATION",
		"jwt.secret":             "GINCHAT_JWT_SECRET",
		"jwt.expire_hours":       "GINCHAT_JWT_EXPIRE_HOURS",
		"redis.host":             "GINCHAT_REDIS_HOST",
		"redis.port":             "GINCHAT_REDIS_PORT",
		"redis.password":         "GINCHAT_REDIS_PASSWORD",
		"redis.db":               "GINCHAT_REDIS_DB",
		"snowflake.workID":       "GINCHAT_SNOWFLAKE_WORK_ID",
	}

	// 显式绑定让环境变量名成为稳定契约，不依赖结构体字段或 YAML 大小写。
	for key, envName := range bindings {
		if err := v.BindEnv(key, envName); err != nil {
			return fmt.Errorf("绑定环境变量 %s 失败: %w", envName, err)
		}
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
	if strings.TrimSpace(jwt.JwtCfg.Secret) == "" {
		return fmt.Errorf("JWT 密钥不能为空")
	}
	if jwt.JwtCfg.ExpireHours <= 0 {
		return fmt.Errorf("JWT 过期时间必须大于 0")
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
