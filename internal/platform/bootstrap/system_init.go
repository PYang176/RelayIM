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
		"mysql.host":                       "GINCHAT_MYSQL_HOST",
		"mysql.port":                       "GINCHAT_MYSQL_PORT",
		"mysql.user":                       "GINCHAT_MYSQL_USER",
		"mysql.password":                   "GINCHAT_MYSQL_PASSWORD",
		"mysql.dbname":                     "GINCHAT_MYSQL_DBNAME",
		"mysql.config.charset":             "GINCHAT_MYSQL_CHARSET",
		"mysql.config.parseTime":           "GINCHAT_MYSQL_PARSE_TIME",
		"mysql.config.loc":                 "GINCHAT_MYSQL_LOCATION",
		"jwt.secret":                       "GINCHAT_JWT_SECRET",
		"jwt.expire_hours":                 "GINCHAT_JWT_EXPIRE_HOURS",
		"redis.host":                       "GINCHAT_REDIS_HOST",
		"redis.port":                       "GINCHAT_REDIS_PORT",
		"redis.password":                   "GINCHAT_REDIS_PASSWORD",
		"redis.db":                         "GINCHAT_REDIS_DB",
		"rabbitmq.url":                     "GINCHAT_RABBITMQ_URL",
		"rabbitmq.publish_timeout_seconds": "GINCHAT_RABBITMQ_PUBLISH_TIMEOUT_SECONDS",
		"rabbitmq.poll_interval_ms":        "GINCHAT_RABBITMQ_POLL_INTERVAL_MS",
		"rabbitmq.batch_size":              "GINCHAT_RABBITMQ_BATCH_SIZE",
		"rabbitmq.max_publish_attempts":    "GINCHAT_RABBITMQ_MAX_PUBLISH_ATTEMPTS",
		"rabbitmq.retry_base_seconds":      "GINCHAT_RABBITMQ_RETRY_BASE_SECONDS",
		"rabbitmq.claim_lease_seconds":     "GINCHAT_RABBITMQ_CLAIM_LEASE_SECONDS",
		"rabbitmq.prefetch":                "GINCHAT_RABBITMQ_PREFETCH",
		"s3.endpoint":                      "GINCHAT_S3_ENDPOINT",
		"s3.access_key":                    "GINCHAT_S3_ACCESS_KEY",
		"s3.secret_key":                    "GINCHAT_S3_SECRET_KEY",
		"s3.bucket":                        "GINCHAT_S3_BUCKET",
		"s3.region":                        "GINCHAT_S3_REGION",
		"s3.use_ssl":                       "GINCHAT_S3_USE_SSL",
		"rate_limit.ws_ticket_global":      "GINCHAT_WS_TICKET_RATE_LIMIT_GLOBAL",
		"rate_limit.ws_ticket_user":        "GINCHAT_WS_TICKET_RATE_LIMIT_USER",
		"rate_limit.ws_ticket_window_ms":   "GINCHAT_WS_TICKET_RATE_LIMIT_WINDOW_MS",
		"snowflake.workID":                 "GINCHAT_SNOWFLAKE_WORK_ID",
	}

	// 显式绑定让环境变量名成为稳定契约，不依赖结构体字段或 YAML 大小写。
	for key, envName := range bindings {
		if err := v.BindEnv(key, envName); err != nil {
			return fmt.Errorf("绑定环境变量 %s 失败: %w", envName, err)
		}
	}
	return nil
}

// LoadWSTicketRateLimitConfig 从 Viper 读取 WebSocket Ticket 限流配置。
func LoadWSTicketRateLimitConfig() config.WSTicketRateLimitConfig {
	return loadWSTicketRateLimitConfig(viper.GetViper())
}

func loadWSTicketRateLimitConfig(
	v *viper.Viper,
) config.WSTicketRateLimitConfig {
	cfg := config.WSTicketRateLimitConfig{
		GlobalLimit:        v.GetInt("rate_limit.ws_ticket_global"),
		UserLimit:          v.GetInt("rate_limit.ws_ticket_user"),
		WindowMilliseconds: v.GetInt("rate_limit.ws_ticket_window_ms"),
	}
	if cfg.GlobalLimit <= 0 {
		cfg.GlobalLimit = 250
	}
	if cfg.UserLimit <= 0 {
		cfg.UserLimit = 10
	}
	if cfg.WindowMilliseconds <= 0 {
		cfg.WindowMilliseconds = 1000
	}
	return cfg
}

// LoadRabbitMQConfig 从 Viper 读取 RabbitMQ 后台任务配置。
func LoadRabbitMQConfig() config.RabbitMQConfig {
	return loadRabbitMQConfig(viper.GetViper())
}

func loadRabbitMQConfig(v *viper.Viper) config.RabbitMQConfig {
	cfg := config.RabbitMQConfig{
		URL:                      strings.TrimSpace(v.GetString("rabbitmq.url")),
		PublishTimeoutSeconds:    v.GetInt("rabbitmq.publish_timeout_seconds"),
		PollIntervalMilliseconds: v.GetInt("rabbitmq.poll_interval_ms"),
		BatchSize:                v.GetInt("rabbitmq.batch_size"),
		MaxPublishAttempts:       v.GetInt("rabbitmq.max_publish_attempts"),
		RetryBaseSeconds:         v.GetInt("rabbitmq.retry_base_seconds"),
		ClaimLeaseSeconds:        v.GetInt("rabbitmq.claim_lease_seconds"),
		Prefetch:                 v.GetInt("rabbitmq.prefetch"),
	}
	if cfg.PublishTimeoutSeconds <= 0 {
		cfg.PublishTimeoutSeconds = 5
	}
	if cfg.PollIntervalMilliseconds <= 0 {
		cfg.PollIntervalMilliseconds = 500
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 100
	}
	if cfg.MaxPublishAttempts <= 0 {
		cfg.MaxPublishAttempts = 10
	}
	if cfg.RetryBaseSeconds <= 0 {
		cfg.RetryBaseSeconds = 2
	}
	if cfg.ClaimLeaseSeconds <= 0 {
		cfg.ClaimLeaseSeconds = 30
	}
	if cfg.Prefetch <= 0 {
		cfg.Prefetch = 20
	}
	return cfg
}

// LoadObjectStorageConfig 从 Viper 读取 SeaweedFS S3 配置。
func LoadObjectStorageConfig() config.ObjectStorageConfig {
	return loadObjectStorageConfig(viper.GetViper())
}

func loadObjectStorageConfig(v *viper.Viper) config.ObjectStorageConfig {
	cfg := config.ObjectStorageConfig{
		Endpoint:  strings.TrimSpace(v.GetString("s3.endpoint")),
		AccessKey: strings.TrimSpace(v.GetString("s3.access_key")),
		SecretKey: strings.TrimSpace(v.GetString("s3.secret_key")),
		Bucket:    strings.TrimSpace(v.GetString("s3.bucket")),
		Region:    strings.TrimSpace(v.GetString("s3.region")),
		UseSSL:    v.GetBool("s3.use_ssl"),
	}
	if cfg.Endpoint == "" {
		cfg.Endpoint = "127.0.0.1:8333"
	}
	if cfg.Bucket == "" {
		cfg.Bucket = "ginchat-media"
	}
	if cfg.Region == "" {
		cfg.Region = "us-east-1"
	}
	return cfg
}

// LoadMySQLConfig 从 Viper 读取 MySQL 配置。
// 逐字段读取可以确保已绑定的 GINCHAT_* 环境变量可靠覆盖 YAML 中的空值。
func LoadMySQLConfig() config.MySQLConfig {
	return loadMySQLConfig(viper.GetViper())
}

func loadMySQLConfig(v *viper.Viper) config.MySQLConfig {
	return config.MySQLConfig{
		Host:     v.GetString("mysql.host"),
		Port:     v.GetInt("mysql.port"),
		User:     v.GetString("mysql.user"),
		Password: v.GetString("mysql.password"),
		DBName:   v.GetString("mysql.dbname"),
		Config: config.MySQLExtraConfig{
			Charset:   v.GetString("mysql.config.charset"),
			ParseTime: v.GetBool("mysql.config.parseTime"),
			Loc:       v.GetString("mysql.config.loc"),
		},
	}
}

// InitMySQL 初始化Mysql数据库连接对象
func InitMySQL(cfg config.MySQLConfig) (*gorm.DB, error) {
	dsn := GetMySQLDsn(cfg)

	// 默认只记录慢 SQL 和错误，避免消息正文、密码哈希等数据进入普通日志。
	newLogger := logger.New(
		log.New(os.Stdout, "\r\n", log.LstdFlags),
		logger.Config{
			SlowThreshold:             time.Second,
			LogLevel:                  logger.Warn,
			Colorful:                  true,
			ParameterizedQueries:      true,
			IgnoreRecordNotFoundError: true,
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
	v := viper.GetViper()
	jwt.JwtCfg.Secret = v.GetString("jwt.secret")
	jwt.JwtCfg.ExpireHours = v.GetInt("jwt.expire_hours")
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
	v := viper.GetViper()
	redisCfg := config.RedisConfig{
		Host:     v.GetString("redis.host"),
		Port:     v.GetInt("redis.port"),
		Password: v.GetString("redis.password"),
		DB:       v.GetInt("redis.db"),
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
