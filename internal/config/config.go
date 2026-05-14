package config

// MySQLConfig MySQL配置结构体
type MySQLConfig struct {
	Host     string           `yaml:"host" json:"host" mapstructure:"host"`
	Port     int              `yaml:"port" json:"port" mapstructure:"port"`
	User     string           `yaml:"user" json:"user" mapstructure:"user"`
	Password string           `yaml:"password" json:"password" mapstructure:"password"`
	DBName   string           `yaml:"dbname" json:"dbname" mapstructure:"dbname"`
	Config   MySQLExtraConfig `yaml:"config" json:"config" mapstructure:"config"`
}

// MySQLExtraConfig 额外配置
type MySQLExtraConfig struct {
	Charset   string `yaml:"charset" json:"charset" mapstructure:"charset"`
	ParseTime bool   `yaml:"parseTime" json:"parseTime" mapstructure:"parseTime"`
	Loc       string `yaml:"loc" json:"loc" mapstructure:"loc"`
}

// JWTConfig JWT令牌
type JWTConfig struct {
	// 密钥
	Secret string `yaml:"secret" mapstructure:"secret"`
	// 过期时间
	ExpireHours int `yaml:"expire_hours" mapstructure:"expire_hours"`
}

// RedisConfig Redis的配置信息
type RedisConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}
