package bootstrap

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/PYang176/RelayIM/internal/config"
	"github.com/PYang176/RelayIM/internal/platform/jwt"
	"github.com/PYang176/RelayIM/internal/platform/redisstore"
	"github.com/PYang176/RelayIM/internal/platform/snowflake"
	"github.com/spf13/viper"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"log"
	"os"
	"strings"
	"time"
)

const maxSnowflakeClockRollbackWait = 5 * time.Second

// InitConfig 初始化配置运行状态
func InitConfig() error {
	v := viper.GetViper()
	if err := bindConfigEnv(v); err != nil {
		return err
	}

	if configFile := strings.TrimSpace(os.Getenv("RELAYIM_CONFIG_FILE")); configFile != "" {
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

// bindConfigEnv 绑定 RELAYIM 环境变量到配置键，保证运行参数覆盖 YAML
func bindConfigEnv(v *viper.Viper) error {
	// 前缀、键名替换和 AutomaticEnv 为后续普通字段提供统一覆盖规则；
	// 下面显式绑定关键字段，保证嵌套 YAML 键的环境变量名稳定可控。
	v.SetEnvPrefix("RELAYIM")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	bindings := map[string]string{
		"mysql.host":                             "RELAYIM_MYSQL_HOST",
		"mysql.port":                             "RELAYIM_MYSQL_PORT",
		"mysql.user":                             "RELAYIM_MYSQL_USER",
		"mysql.password":                         "RELAYIM_MYSQL_PASSWORD",
		"mysql.dbname":                           "RELAYIM_MYSQL_DBNAME",
		"mysql.config.charset":                   "RELAYIM_MYSQL_CHARSET",
		"mysql.config.parseTime":                 "RELAYIM_MYSQL_PARSE_TIME",
		"mysql.config.loc":                       "RELAYIM_MYSQL_LOCATION",
		"jwt.secret":                             "RELAYIM_JWT_SECRET",
		"jwt.expire_hours":                       "RELAYIM_JWT_EXPIRE_HOURS",
		"jwt.refresh_expire_days":                "RELAYIM_JWT_REFRESH_EXPIRE_DAYS",
		"redis.host":                             "RELAYIM_REDIS_HOST",
		"redis.port":                             "RELAYIM_REDIS_PORT",
		"redis.password":                         "RELAYIM_REDIS_PASSWORD",
		"redis.db":                               "RELAYIM_REDIS_DB",
		"rabbitmq.url":                           "RELAYIM_RABBITMQ_URL",
		"rabbitmq.publish_timeout_seconds":       "RELAYIM_RABBITMQ_PUBLISH_TIMEOUT_SECONDS",
		"rabbitmq.poll_interval_ms":              "RELAYIM_RABBITMQ_POLL_INTERVAL_MS",
		"rabbitmq.batch_size":                    "RELAYIM_RABBITMQ_BATCH_SIZE",
		"rabbitmq.max_publish_attempts":          "RELAYIM_RABBITMQ_MAX_PUBLISH_ATTEMPTS",
		"rabbitmq.retry_base_seconds":            "RELAYIM_RABBITMQ_RETRY_BASE_SECONDS",
		"rabbitmq.claim_lease_seconds":           "RELAYIM_RABBITMQ_CLAIM_LEASE_SECONDS",
		"rabbitmq.prefetch":                      "RELAYIM_RABBITMQ_PREFETCH",
		"s3.endpoint":                            "RELAYIM_S3_ENDPOINT",
		"s3.public_endpoint":                     "RELAYIM_S3_PUBLIC_ENDPOINT",
		"s3.access_key":                          "RELAYIM_S3_ACCESS_KEY",
		"s3.secret_key":                          "RELAYIM_S3_SECRET_KEY",
		"s3.bucket":                              "RELAYIM_S3_BUCKET",
		"s3.region":                              "RELAYIM_S3_REGION",
		"s3.use_ssl":                             "RELAYIM_S3_USE_SSL",
		"rate_limit.ws_ticket_global":            "RELAYIM_WS_TICKET_RATE_LIMIT_GLOBAL",
		"rate_limit.ws_ticket_user":              "RELAYIM_WS_TICKET_RATE_LIMIT_USER",
		"rate_limit.ws_ticket_window_ms":         "RELAYIM_WS_TICKET_RATE_LIMIT_WINDOW_MS",
		"snowflake.workID":                       "RELAYIM_SNOWFLAKE_WORK_ID",
		"tracing.enabled":                        "RELAYIM_TRACING_ENABLED",
		"tracing.service_name":                   "RELAYIM_TRACING_SERVICE_NAME",
		"tracing.otlp_endpoint":                  "RELAYIM_TRACING_OTLP_ENDPOINT",
		"tracing.sample_ratio":                   "RELAYIM_TRACING_SAMPLE_RATIO",
		"attachment_cleanup.enabled":             "RELAYIM_ATTACHMENT_CLEANUP_ENABLED",
		"attachment_cleanup.interval_seconds":    "RELAYIM_ATTACHMENT_CLEANUP_INTERVAL_SECONDS",
		"attachment_cleanup.pending_ttl_hours":   "RELAYIM_ATTACHMENT_CLEANUP_PENDING_TTL_HOURS",
		"attachment_cleanup.claim_lease_seconds": "RELAYIM_ATTACHMENT_CLEANUP_CLAIM_LEASE_SECONDS",
		"attachment_cleanup.batch_size":          "RELAYIM_ATTACHMENT_CLEANUP_BATCH_SIZE",
	}

	// 显式绑定让环境变量名成为稳定契约，不依赖结构体字段或 YAML 大小写。
	for key, envName := range bindings {
		if err := v.BindEnv(key, envName); err != nil {
			return fmt.Errorf("绑定环境变量 %s 失败: %w", envName, err)
		}
	}
	return nil
}

// LoadTracingConfig 从配置中心读取 OpenTelemetry Trace 参数。
func LoadTracingConfig() config.TracingConfig {
	return loadTracingConfig(viper.GetViper())
}

// loadTracingConfig 加载Tracing配置所需数据
func loadTracingConfig(v *viper.Viper) config.TracingConfig {
	cfg := config.TracingConfig{
		Enabled:      v.GetBool("tracing.enabled"),
		ServiceName:  strings.TrimSpace(v.GetString("tracing.service_name")),
		OTLPEndpoint: strings.TrimSpace(v.GetString("tracing.otlp_endpoint")),
		SampleRatio:  v.GetFloat64("tracing.sample_ratio"),
	}
	if cfg.ServiceName == "" {
		cfg.ServiceName = "relayim"
	}
	if cfg.OTLPEndpoint == "" {
		cfg.OTLPEndpoint = "http://127.0.0.1:4318"
	}
	if cfg.SampleRatio <= 0 || cfg.SampleRatio > 1 {
		cfg.SampleRatio = 1
	}
	return cfg
}

// LoadAttachmentCleanupConfig 读取未绑定附件清理任务配置。
func LoadAttachmentCleanupConfig() config.AttachmentCleanupConfig {
	return loadAttachmentCleanupConfig(viper.GetViper())
}

// loadAttachmentCleanupConfig 加载附件清理配置所需数据
func loadAttachmentCleanupConfig(
	v *viper.Viper,
) config.AttachmentCleanupConfig {
	cfg := config.AttachmentCleanupConfig{
		Enabled:           v.GetBool("attachment_cleanup.enabled"),
		IntervalSeconds:   v.GetInt("attachment_cleanup.interval_seconds"),
		PendingTTLHours:   v.GetInt("attachment_cleanup.pending_ttl_hours"),
		ClaimLeaseSeconds: v.GetInt("attachment_cleanup.claim_lease_seconds"),
		BatchSize:         v.GetInt("attachment_cleanup.batch_size"),
	}
	if cfg.IntervalSeconds <= 0 {
		cfg.IntervalSeconds = 3600
	}
	if cfg.PendingTTLHours <= 0 {
		cfg.PendingTTLHours = 24
	}
	if cfg.ClaimLeaseSeconds <= 0 {
		cfg.ClaimLeaseSeconds = 600
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 100
	}
	return cfg
}

// LoadWSTicketRateLimitConfig 从 Viper 读取 WebSocket Ticket 限流配置。
func LoadWSTicketRateLimitConfig() config.WSTicketRateLimitConfig {
	return loadWSTicketRateLimitConfig(viper.GetViper())
}

// loadWSTicketRateLimitConfig 加载WS票据速率限制配置所需数据
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

// loadRabbitMQConfig 加载RabbitMQ配置所需数据
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

// loadObjectStorageConfig 加载对象存储配置所需数据
func loadObjectStorageConfig(v *viper.Viper) config.ObjectStorageConfig {
	cfg := config.ObjectStorageConfig{
		Endpoint:       strings.TrimSpace(v.GetString("s3.endpoint")),
		PublicEndpoint: strings.TrimSpace(v.GetString("s3.public_endpoint")),
		AccessKey:      strings.TrimSpace(v.GetString("s3.access_key")),
		SecretKey:      strings.TrimSpace(v.GetString("s3.secret_key")),
		Bucket:         strings.TrimSpace(v.GetString("s3.bucket")),
		Region:         strings.TrimSpace(v.GetString("s3.region")),
		UseSSL:         v.GetBool("s3.use_ssl"),
	}
	if cfg.Endpoint == "" {
		cfg.Endpoint = "127.0.0.1:8333"
	}
	if cfg.Bucket == "" {
		cfg.Bucket = "relayim-media"
	}
	if cfg.Region == "" {
		cfg.Region = "us-east-1"
	}
	return cfg
}

// LoadMySQLConfig 从 Viper 读取 MySQL 配置。
// 逐字段读取可以确保已绑定的 RELAYIM_* 环境变量可靠覆盖 YAML 中的空值。
func LoadMySQLConfig() config.MySQLConfig {
	return loadMySQLConfig(viper.GetViper())
}

// loadMySQLConfig 加载MySQL配置所需数据
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
	jwt.JwtCfg.RefreshExpireDays = v.GetInt("jwt.refresh_expire_days")
	if strings.TrimSpace(jwt.JwtCfg.Secret) == "" {
		return fmt.Errorf("JWT 密钥不能为空")
	}
	if jwt.JwtCfg.ExpireHours <= 0 {
		return fmt.Errorf("JWT 过期时间必须大于 0")
	}
	if jwt.JwtCfg.RefreshExpireDays <= 0 {
		jwt.JwtCfg.RefreshExpireDays = 30
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

// InitSnowFlake 校验数据库时间水位后初始化雪花算法。
func InitSnowFlake(ctx context.Context, db *gorm.DB) error {
	workID := viper.GetInt64("snowflake.workID")
	latest, err := latestSnowflakeTime(ctx, db)
	if err != nil {
		return err
	}
	if err := ensureSnowflakeClockAhead(
		ctx,
		latest,
		time.Now,
		maxSnowflakeClockRollbackWait,
	); err != nil {
		return err
	}
	if err := snowflake.Init(workID); err != nil {
		return fmt.Errorf("初始化 Snowflake 失败: %w", err)
	}
	return nil
}

// latestSnowflakeTime 查询数据库中已有雪花 ID 的最大时间水位
func latestSnowflakeTime(
	ctx context.Context,
	db *gorm.DB,
) (time.Time, error) {
	query := `
SELECT MAX(snowflake_value)
FROM (
    SELECT MAX(id) AS snowflake_value FROM users
    UNION ALL SELECT MAX(id) FROM devices
    UNION ALL SELECT MAX(id) FROM refresh_tokens
    UNION ALL SELECT MAX(id) FROM friend_requests
    UNION ALL SELECT MAX(id) FROM friendships
    UNION ALL SELECT MAX(id) FROM conversations
    UNION ALL SELECT MAX(id) FROM group_invitations
    UNION ALL SELECT MAX(id) FROM messages
    UNION ALL SELECT MAX(id) FROM attachments
    UNION ALL SELECT MAX(event_id) FROM outbox_events
    UNION ALL SELECT MAX(event_id) FROM processed_events
) AS generated_ids`
	var maxID sql.NullInt64
	if err := db.WithContext(ctx).Raw(query).Scan(&maxID).Error; err != nil {
		return time.Time{}, fmt.Errorf("查询雪花 ID 时间水位: %w", err)
	}
	if !maxID.Valid || maxID.Int64 <= 0 {
		return time.Time{}, nil
	}
	return snowflake.Timestamp(maxID.Int64), nil
}

// ensureSnowflakeClockAhead 校验系统时钟是否追平雪花时间水位
func ensureSnowflakeClockAhead(
	ctx context.Context,
	latest time.Time,
	now func() time.Time,
	maxWait time.Duration,
) error {
	if latest.IsZero() {
		return nil
	}
	target := latest.Add(time.Millisecond)
	wait := target.Sub(now())
	if wait <= 0 {
		return nil
	}
	if wait > maxWait {
		return fmt.Errorf(
			"系统时钟落后数据库雪花时间水位 %s，拒绝启动；请先校准 NTP/chrony",
			wait.Round(time.Millisecond),
		)
	}

	log.Printf("检测到雪花时钟小幅回拨，等待 %s 追平", wait.Round(time.Millisecond))
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return fmt.Errorf("等待雪花时钟追平被取消: %w", ctx.Err())
	case <-timer.C:
	}
	if now().Before(target) {
		return fmt.Errorf("系统时钟未在等待后追平雪花时间水位，拒绝启动")
	}
	return nil
}
