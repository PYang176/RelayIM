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
	// RefreshExpireDays 是 Refresh Token 的有效天数。
	RefreshExpireDays int `yaml:"refresh_expire_days" mapstructure:"refresh_expire_days"`
}

// RedisConfig Redis的配置信息
type RedisConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

// RabbitMQConfig 控制 Outbox 发布器和消息消费者的连接与重试行为。
type RabbitMQConfig struct {
	// URL 是完整 AMQP 连接地址；为空时 RabbitMQ 后台任务保持禁用。
	URL string `mapstructure:"url"`

	// PublishTimeoutSeconds 是单条事件等待 Publisher Confirm 的最长时间。
	PublishTimeoutSeconds int `mapstructure:"publish_timeout_seconds"`

	// PollIntervalMilliseconds 是 Outbox 无任务时的扫描间隔。
	PollIntervalMilliseconds int `mapstructure:"poll_interval_ms"`

	// BatchSize 是单次最多领取的 Outbox 事件数量。
	BatchSize int `mapstructure:"batch_size"`

	// MaxPublishAttempts 是 Outbox 事件进入死信状态前允许的最大发布次数。
	MaxPublishAttempts int `mapstructure:"max_publish_attempts"`

	// RetryBaseSeconds 是指数退避的初始秒数。
	RetryBaseSeconds int `mapstructure:"retry_base_seconds"`

	// ClaimLeaseSeconds 是发布实例持有事件的租约时间。
	ClaimLeaseSeconds int `mapstructure:"claim_lease_seconds"`

	// Prefetch 限制单个消费者尚未 ACK 的消息数量。
	Prefetch int `mapstructure:"prefetch"`
}

// ObjectStorageConfig 控制 SeaweedFS 的 S3 兼容对象存储。
type ObjectStorageConfig struct {
	// Endpoint 是 S3 API 的 host:port，不包含协议前缀。
	Endpoint string `mapstructure:"endpoint"`

	// PublicEndpoint 是浏览器访问预签名对象的完整 URL，容器部署时与 Endpoint 分离。
	PublicEndpoint string `mapstructure:"public_endpoint"`

	// AccessKey 是 S3 访问密钥。
	AccessKey string `mapstructure:"access_key"`

	// SecretKey 是 S3 访问密钥对应的秘密。
	SecretKey string `mapstructure:"secret_key"`

	// Bucket 保存聊天图片和文件。
	Bucket string `mapstructure:"bucket"`

	// Region 是 S3 客户端签名使用的区域名称。
	Region string `mapstructure:"region"`

	// UseSSL 表示是否使用 HTTPS 连接对象存储。
	UseSSL bool `mapstructure:"use_ssl"`
}

// WSTicketRateLimitConfig 控制 WebSocket Ticket 的瞬时请求速率。
type WSTicketRateLimitConfig struct {
	// GlobalLimit 是滑动时间窗口内全进程共享的 Ticket 请求上限。
	GlobalLimit int `mapstructure:"global_limit"`

	// UserLimit 是单个用户在滑动时间窗口内的 Ticket 请求上限。
	UserLimit int `mapstructure:"user_limit"`

	// WindowMilliseconds 是滑动限流窗口长度。
	WindowMilliseconds int `mapstructure:"window_ms"`
}

// TracingConfig 控制 OpenTelemetry Trace 的采样和 OTLP 导出。
type TracingConfig struct {
	// Enabled 表示是否启用 Trace；关闭时使用无操作 Provider。
	Enabled bool `mapstructure:"enabled"`

	// ServiceName 是 Trace 中展示的服务名称。
	ServiceName string `mapstructure:"service_name"`

	// OTLPEndpoint 是 OTLP HTTP Trace 接收地址。
	OTLPEndpoint string `mapstructure:"otlp_endpoint"`

	// SampleRatio 是根 Span 的采样比例，范围 0 到 1。
	SampleRatio float64 `mapstructure:"sample_ratio"`
}

// AttachmentCleanupConfig 控制未绑定附件的后台清理。
type AttachmentCleanupConfig struct {
	// Enabled 表示是否启动清理任务。
	Enabled bool `mapstructure:"enabled"`

	// IntervalSeconds 是两轮清理之间的间隔。
	IntervalSeconds int `mapstructure:"interval_seconds"`

	// PendingTTLHours 是待确认附件进入清理前保留的小时数。
	PendingTTLHours int `mapstructure:"pending_ttl_hours"`

	// ClaimLeaseSeconds 是清理中状态的最长租约，用于回收崩溃任务。
	ClaimLeaseSeconds int `mapstructure:"claim_lease_seconds"`

	// BatchSize 是单轮最多清理的附件数。
	BatchSize int `mapstructure:"batch_size"`
}
