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
