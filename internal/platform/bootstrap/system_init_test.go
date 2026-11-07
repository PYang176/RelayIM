package bootstrap

import (
	"testing"

	"github.com/spf13/viper"
)

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
