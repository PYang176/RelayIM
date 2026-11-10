package bootstrap

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"ginChatProject/internal/config"
	"ginChatProject/internal/platform/snowflake"
	"github.com/spf13/viper"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// TestEnsureSnowflakeClockAhead 覆盖雪花时间水位回拨等待与拒绝启动
func TestEnsureSnowflakeClockAhead(t *testing.T) {
	base := time.Date(2026, time.November, 9, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name    string
		latest  time.Time
		now     func() time.Time
		maxWait time.Duration
		wantErr bool
	}{
		{
			name: "数据库没有雪花 ID",
			now:  func() time.Time { return base },
		},
		{
			name:   "当前时钟领先数据库",
			latest: base.Add(-time.Second),
			now:    func() time.Time { return base },
		},
		{
			name:    "小幅回拨可等待追平",
			latest:  time.Now().Add(5 * time.Millisecond),
			now:     time.Now,
			maxWait: 100 * time.Millisecond,
		},
		{
			name:    "大幅回拨拒绝启动",
			latest:  base,
			now:     func() time.Time { return base.Add(-time.Minute) },
			maxWait: 5 * time.Second,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ensureSnowflakeClockAhead(
				context.Background(),
				tt.latest,
				tt.now,
				tt.maxWait,
			)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ensureSnowflakeClockAhead() 错误 = %v", err)
			}
		})
	}
}

// TestEnsureSnowflakeClockAheadCanceled 覆盖等待雪花时钟追平时的上下文取消
func TestEnsureSnowflakeClockAheadCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := ensureSnowflakeClockAhead(
		ctx,
		time.Now().Add(20*time.Millisecond),
		time.Now,
		100*time.Millisecond,
	)
	if err == nil {
		t.Fatal("取消等待应返回错误")
	}
}

// TestLatestSnowflakeTimeIntegration 覆盖从真实数据库读取雪花时间水位
func TestLatestSnowflakeTimeIntegration(t *testing.T) {
	dsn := os.Getenv("GINCHAT_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("未设置 GINCHAT_TEST_MYSQL_DSN，跳过 MySQL 雪花时间水位测试")
	}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("连接测试数据库失败: %v", err)
	}
	if err := snowflake.Init(1); err != nil {
		t.Fatalf("初始化雪花生成器失败: %v", err)
	}
	userID := uint64(snowflake.Generate())
	username := fmt.Sprintf("sf_probe_%d", userID)
	if err := db.Exec(
		`INSERT INTO users (id, username, password_hash, status)
		 VALUES (?, ?, 'not-used', 1)`,
		userID,
		username,
	).Error; err != nil {
		t.Fatalf("创建雪花时间水位测试用户失败: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Exec("DELETE FROM users WHERE id = ?", userID).Error; err != nil {
			t.Errorf("清理雪花时间水位测试用户失败: %v", err)
		}
	})

	latest, err := latestSnowflakeTime(context.Background(), db)
	if err != nil {
		t.Fatalf("查询雪花时间水位失败: %v", err)
	}
	if latest.IsZero() {
		t.Fatal("插入雪花记录后应返回非零时间水位")
	}
	if latest.After(time.Now().Add(5 * time.Second)) {
		t.Fatalf("雪花时间水位异常超前: %v", latest)
	}
}

// TestBindConfigEnv 覆盖 GINCHAT 环境变量到配置键的绑定
func TestBindConfigEnv(t *testing.T) {
	tests := []struct {
		name    string
		envName string
		envVal  string
		key     string
		want    any
	}{
		{
			name:    "MySQL 密码覆盖",
			envName: "GINCHAT_MYSQL_PASSWORD",
			envVal:  "local-password",
			key:     "mysql.password",
			want:    "local-password",
		},
		{
			name:    "数据库名覆盖",
			envName: "GINCHAT_MYSQL_DBNAME",
			envVal:  "ginchat_v2",
			key:     "mysql.dbname",
			want:    "ginchat_v2",
		},
		{
			name:    "布尔配置覆盖",
			envName: "GINCHAT_MYSQL_PARSE_TIME",
			envVal:  "false",
			key:     "mysql.config.parseTime",
			want:    false,
		},
		{
			name:    "Snowflake worker ID 覆盖",
			envName: "GINCHAT_SNOWFLAKE_WORK_ID",
			envVal:  "7",
			key:     "snowflake.workID",
			want:    int64(7),
		},
		{
			name:    "RabbitMQ 地址覆盖",
			envName: "GINCHAT_RABBITMQ_URL",
			envVal:  "amqp://user:password@127.0.0.1:5672/ginchat",
			key:     "rabbitmq.url",
			want:    "amqp://user:password@127.0.0.1:5672/ginchat",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(tt.envName, tt.envVal)
			v := viper.New()
			if err := bindConfigEnv(v); err != nil {
				t.Fatalf("绑定环境变量失败: %v", err)
			}

			switch tt.want.(type) {
			case bool:
				if got := v.GetBool(tt.key); got != tt.want {
					t.Fatalf("%s = %v，期望 %v", tt.key, got, tt.want)
				}
			case int64:
				if got := v.GetInt64(tt.key); got != tt.want {
					t.Fatalf("%s = %v，期望 %v", tt.key, got, tt.want)
				}
			default:
				if got := v.GetString(tt.key); got != tt.want {
					t.Fatalf("%s = %q，期望 %q", tt.key, got, tt.want)
				}
			}
		})
	}
}

// TestLoadRabbitMQConfigFromEnvironment 覆盖 RabbitMQ 配置的环境变量覆盖和默认值
func TestLoadRabbitMQConfigFromEnvironment(t *testing.T) {
	t.Setenv("GINCHAT_RABBITMQ_URL", "amqp://user:password@127.0.0.1:5672/ginchat")
	t.Setenv("GINCHAT_RABBITMQ_PUBLISH_TIMEOUT_SECONDS", "6")
	t.Setenv("GINCHAT_RABBITMQ_POLL_INTERVAL_MS", "250")
	t.Setenv("GINCHAT_RABBITMQ_BATCH_SIZE", "50")
	t.Setenv("GINCHAT_RABBITMQ_MAX_PUBLISH_ATTEMPTS", "8")
	t.Setenv("GINCHAT_RABBITMQ_RETRY_BASE_SECONDS", "3")
	t.Setenv("GINCHAT_RABBITMQ_CLAIM_LEASE_SECONDS", "45")
	t.Setenv("GINCHAT_RABBITMQ_PREFETCH", "30")

	v := viper.New()
	if err := bindConfigEnv(v); err != nil {
		t.Fatalf("绑定环境变量失败: %v", err)
	}

	cfg := loadRabbitMQConfig(v)
	if cfg.URL != "amqp://user:password@127.0.0.1:5672/ginchat" ||
		cfg.PublishTimeoutSeconds != 6 ||
		cfg.PollIntervalMilliseconds != 250 ||
		cfg.BatchSize != 50 ||
		cfg.MaxPublishAttempts != 8 ||
		cfg.RetryBaseSeconds != 3 ||
		cfg.ClaimLeaseSeconds != 45 ||
		cfg.Prefetch != 30 {
		t.Fatalf("LoadRabbitMQConfig() = %+v，环境变量没有正确覆盖默认值", cfg)
	}
}

// TestLoadTracingConfigFromEnvironment 覆盖 Trace 配置的环境变量覆盖和采样边界
func TestLoadTracingConfigFromEnvironment(t *testing.T) {
	t.Setenv("GINCHAT_TRACING_ENABLED", "true")
	t.Setenv("GINCHAT_TRACING_SERVICE_NAME", "ginchat-test")
	t.Setenv("GINCHAT_TRACING_OTLP_ENDPOINT", "http://127.0.0.1:4319")
	t.Setenv("GINCHAT_TRACING_SAMPLE_RATIO", "0.25")

	v := viper.New()
	if err := bindConfigEnv(v); err != nil {
		t.Fatalf("绑定环境变量失败: %v", err)
	}
	cfg := loadTracingConfig(v)
	expected := config.TracingConfig{
		Enabled:      true,
		ServiceName:  "ginchat-test",
		OTLPEndpoint: "http://127.0.0.1:4319",
		SampleRatio:  0.25,
	}
	if cfg != expected {
		t.Fatalf("LoadTracingConfig() = %+v，期望 %+v", cfg, expected)
	}
}

// TestLoadAttachmentCleanupConfigFromEnvironment 覆盖附件清理配置的环境变量覆盖和默认值
func TestLoadAttachmentCleanupConfigFromEnvironment(t *testing.T) {
	t.Setenv("GINCHAT_ATTACHMENT_CLEANUP_ENABLED", "true")
	t.Setenv("GINCHAT_ATTACHMENT_CLEANUP_INTERVAL_SECONDS", "60")
	t.Setenv("GINCHAT_ATTACHMENT_CLEANUP_PENDING_TTL_HOURS", "12")
	t.Setenv("GINCHAT_ATTACHMENT_CLEANUP_CLAIM_LEASE_SECONDS", "120")
	t.Setenv("GINCHAT_ATTACHMENT_CLEANUP_BATCH_SIZE", "25")

	v := viper.New()
	if err := bindConfigEnv(v); err != nil {
		t.Fatalf("绑定环境变量失败: %v", err)
	}
	cfg := loadAttachmentCleanupConfig(v)
	expected := config.AttachmentCleanupConfig{
		Enabled:           true,
		IntervalSeconds:   60,
		PendingTTLHours:   12,
		ClaimLeaseSeconds: 120,
		BatchSize:         25,
	}
	if cfg != expected {
		t.Fatalf(
			"LoadAttachmentCleanupConfig() = %+v，期望 %+v",
			cfg,
			expected,
		)
	}
}

// TestLoadObjectStorageConfigFromEnvironment 覆盖对象存储配置的环境变量覆盖和默认值
func TestLoadObjectStorageConfigFromEnvironment(t *testing.T) {
	t.Setenv("GINCHAT_S3_ENDPOINT", "127.0.0.1:8333")
	t.Setenv("GINCHAT_S3_ACCESS_KEY", "test-access")
	t.Setenv("GINCHAT_S3_SECRET_KEY", "test-secret")
	t.Setenv("GINCHAT_S3_BUCKET", "ginchat-test")
	t.Setenv("GINCHAT_S3_REGION", "us-east-1")
	t.Setenv("GINCHAT_S3_USE_SSL", "false")

	v := viper.New()
	if err := bindConfigEnv(v); err != nil {
		t.Fatalf("绑定环境变量失败: %v", err)
	}

	cfg := loadObjectStorageConfig(v)
	if cfg.Endpoint != "127.0.0.1:8333" ||
		cfg.AccessKey != "test-access" ||
		cfg.SecretKey != "test-secret" ||
		cfg.Bucket != "ginchat-test" ||
		cfg.Region != "us-east-1" ||
		cfg.UseSSL {
		t.Fatalf("LoadObjectStorageConfig() = %+v", cfg)
	}
}

// TestLoadWSTicketRateLimitConfigFromEnvironment 覆盖 Ticket 限流配置的环境变量覆盖
func TestLoadWSTicketRateLimitConfigFromEnvironment(t *testing.T) {
	t.Setenv("GINCHAT_WS_TICKET_RATE_LIMIT_GLOBAL", "500")
	t.Setenv("GINCHAT_WS_TICKET_RATE_LIMIT_USER", "20")
	t.Setenv("GINCHAT_WS_TICKET_RATE_LIMIT_WINDOW_MS", "2000")

	v := viper.New()
	if err := bindConfigEnv(v); err != nil {
		t.Fatalf("绑定环境变量失败: %v", err)
	}

	cfg := loadWSTicketRateLimitConfig(v)
	expected := (config.WSTicketRateLimitConfig{
		GlobalLimit:        500,
		UserLimit:          20,
		WindowMilliseconds: 2000,
	})
	if cfg != expected {
		t.Fatalf("LoadWSTicketRateLimitConfig() = %+v，期望 %+v", cfg, expected)
	}
}

// TestLoadMySQLConfigFromEnvironment 覆盖 MySQL 配置的环境变量覆盖
func TestLoadMySQLConfigFromEnvironment(t *testing.T) {
	t.Setenv("GINCHAT_MYSQL_HOST", "127.0.0.1")
	t.Setenv("GINCHAT_MYSQL_PORT", "3307")
	t.Setenv("GINCHAT_MYSQL_USER", "test-user")
	t.Setenv("GINCHAT_MYSQL_PASSWORD", "test-password")
	t.Setenv("GINCHAT_MYSQL_DBNAME", "ginchat_v2")
	t.Setenv("GINCHAT_MYSQL_CHARSET", "utf8mb4")
	t.Setenv("GINCHAT_MYSQL_PARSE_TIME", "true")
	t.Setenv("GINCHAT_MYSQL_LOCATION", "UTC")

	v := viper.New()
	if err := bindConfigEnv(v); err != nil {
		t.Fatalf("绑定环境变量失败: %v", err)
	}

	cfg := loadMySQLConfig(v)
	if cfg.Host != "127.0.0.1" ||
		cfg.Port != 3307 ||
		cfg.User != "test-user" ||
		cfg.Password != "test-password" ||
		cfg.DBName != "ginchat_v2" ||
		cfg.Config.Charset != "utf8mb4" ||
		!cfg.Config.ParseTime ||
		cfg.Config.Loc != "UTC" {
		t.Fatalf("LoadMySQLConfig() = %+v，环境变量没有正确覆盖 YAML 默认值", cfg)
	}
}
