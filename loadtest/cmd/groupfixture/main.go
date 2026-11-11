package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/PYang176/RelayIM/internal/platform/authn"
	"github.com/PYang176/RelayIM/internal/platform/bootstrap"
	"github.com/PYang176/RelayIM/internal/platform/security"
	"github.com/PYang176/RelayIM/internal/platform/snowflake"
	"gorm.io/gorm"
)

const fixturePassword = "Load-test-123456"

type fixtureUser struct {
	ID           uint64 `gorm:"column:id;primaryKey;autoIncrement:false"`
	Username     string `gorm:"column:username"`
	PasswordHash string `gorm:"column:password_hash"`
	Status       int    `gorm:"column:status"`
}

// TableName 返回压测用户写入的表名
func (fixtureUser) TableName() string {
	return "users"
}

type fixtureProfile struct {
	UserID   uint64 `gorm:"column:user_id;primaryKey"`
	Nickname string `gorm:"column:nickname"`
}

// TableName 返回压测用户资料写入的表名
func (fixtureProfile) TableName() string {
	return "user_profiles"
}

type fixtureDevice struct {
	ID         uint64 `gorm:"column:id;primaryKey;autoIncrement:false"`
	UserID     uint64 `gorm:"column:user_id"`
	Platform   int    `gorm:"column:platform"`
	DeviceName string `gorm:"column:device_name"`
}

// TableName 返回压测设备写入的表名
func (fixtureDevice) TableName() string {
	return "devices"
}

type fixtureConversation struct {
	ID               uint64 `gorm:"column:id;primaryKey;autoIncrement:false"`
	ConversationType int    `gorm:"column:conversation_type"`
	OwnerID          uint64 `gorm:"column:owner_id"`
	Name             string `gorm:"column:name"`
	Status           int    `gorm:"column:status"`
	LastSeq          uint64 `gorm:"column:last_seq"`
}

// TableName 返回压测群会话写入的表名
func (fixtureConversation) TableName() string {
	return "conversations"
}

type fixtureMember struct {
	ConversationID uint64 `gorm:"column:conversation_id;primaryKey"`
	UserID         uint64 `gorm:"column:user_id;primaryKey"`
	Role           int    `gorm:"column:role"`
}

// TableName 返回压测群成员写入的表名
func (fixtureMember) TableName() string {
	return "conversation_members"
}

type fixtureMemberResult struct {
	UserID   string `json:"user_id"`
	DeviceID string `json:"device_id"`
	Token    string `json:"token"`
}

type fixtureResult struct {
	RunID          string                `json:"run_id"`
	ConversationID string                `json:"conversation_id"`
	Members        []fixtureMemberResult `json:"members"`
}

// main 创建同群容量压测账号、设备、群会话和 Redis Token 白名单
func main() {
	memberCount := flag.Int("members", 200, "待创建的群成员数量")
	appEnvFile := flag.String("app-env", "internal/config/app.env", "应用环境变量文件")
	configFile := flag.String("config", "internal/config/app.yml", "应用 YAML 配置文件")
	mysqlAddr := flag.String("mysql-addr", "127.0.0.1:3307", "MySQL 地址")
	redisAddr := flag.String("redis-addr", "127.0.0.1:6379", "Redis 地址")
	outputFile := flag.String(
		"output",
		".tmp/group-capacity-fixture.json",
		"包含压测凭据的本地夹具文件",
	)
	flag.Parse()

	if err := run(
		*memberCount,
		*appEnvFile,
		*configFile,
		*mysqlAddr,
		*redisAddr,
		*outputFile,
	); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// run 完成本地配置加载、夹具写入和受控 Token 签发
func run(
	memberCount int,
	appEnvFile string,
	configFile string,
	mysqlAddr string,
	redisAddr string,
	outputFile string,
) error {
	if memberCount < 2 || memberCount > 500 {
		return errors.New("群成员数量必须在 2 到 500 之间")
	}
	if err := loadEnvFile(appEnvFile); err != nil {
		return err
	}
	if err := configureAddresses(mysqlAddr, redisAddr); err != nil {
		return err
	}
	if err := os.Setenv("RELAYIM_CONFIG_FILE", configFile); err != nil {
		return fmt.Errorf("设置配置文件路径: %w", err)
	}
	// 压测夹具使用独立 Worker ID，避免与正在运行的服务生成相同 Snowflake ID
	if err := os.Setenv("RELAYIM_SNOWFLAKE_WORK_ID", "2"); err != nil {
		return fmt.Errorf("设置压测 Snowflake Worker ID: %w", err)
	}

	if err := bootstrap.InitConfig(); err != nil {
		return err
	}
	if err := bootstrap.InitJWT(); err != nil {
		return err
	}
	if err := bootstrap.InitRedis(); err != nil {
		return err
	}
	db, err := bootstrap.InitMySQL(bootstrap.LoadMySQLConfig())
	if err != nil {
		return fmt.Errorf("初始化 MySQL: %w", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("读取 MySQL 连接池: %w", err)
	}
	defer func() {
		if closeErr := sqlDB.Close(); closeErr != nil {
			fmt.Fprintf(os.Stderr, "关闭 MySQL 连接池失败: %v\n", closeErr)
		}
	}()
	if err := snowflake.Init(2); err != nil {
		return fmt.Errorf("初始化压测 Snowflake: %w", err)
	}

	runID, err := newRunID()
	if err != nil {
		return err
	}
	result, err := createFixture(db, memberCount, runID)
	if err != nil {
		return err
	}
	if err := issueTokens(context.Background(), result.Members); err != nil {
		return err
	}
	if err := writeFixture(outputFile, result); err != nil {
		return err
	}

	fmt.Printf(
		"容量夹具已创建: group=%s members=%d output=%s\n",
		result.ConversationID,
		len(result.Members),
		outputFile,
	)
	return nil
}

// loadEnvFile 把 app.env 中尚未设置的变量注入当前夹具进程
func loadEnvFile(path string) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("读取应用环境变量文件: %w", err)
	}
	for _, line := range strings.Split(string(content), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		if key == "" {
			continue
		}
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		if err := os.Setenv(key, value); err != nil {
			return fmt.Errorf("设置环境变量 %s: %w", key, err)
		}
	}
	return nil
}

// configureAddresses 从监听地址中拆出 MySQL 和 Redis 的主机与端口
func configureAddresses(mysqlAddr string, redisAddr string) error {
	mysqlHost, mysqlPort, err := net.SplitHostPort(mysqlAddr)
	if err != nil {
		return fmt.Errorf("解析 MySQL 地址: %w", err)
	}
	redisHost, redisPort, err := net.SplitHostPort(redisAddr)
	if err != nil {
		return fmt.Errorf("解析 Redis 地址: %w", err)
	}
	envValues := map[string]string{
		"RELAYIM_MYSQL_HOST": mysqlHost,
		"RELAYIM_MYSQL_PORT": mysqlPort,
		"RELAYIM_REDIS_HOST": redisHost,
		"RELAYIM_REDIS_PORT": redisPort,
	}
	for key, value := range envValues {
		if err := os.Setenv(key, value); err != nil {
			return fmt.Errorf("设置环境变量 %s: %w", key, err)
		}
	}
	return nil
}

// newRunID 生成适合用户名与清理前缀的短运行标识
func newRunID() (string, error) {
	randomBytes := make([]byte, 3)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", fmt.Errorf("生成压测运行标识: %w", err)
	}
	return time.Now().UTC().Format("150405") + hex.EncodeToString(randomBytes), nil
}

// createFixture 在一个事务中创建容量测试所需的用户、设备和群成员
func createFixture(
	db *gorm.DB,
	memberCount int,
	runID string,
) (fixtureResult, error) {
	passwordHash, err := security.HashPassword(fixturePassword)
	if err != nil {
		return fixtureResult{}, fmt.Errorf("生成压测密码哈希: %w", err)
	}

	users := make([]fixtureUser, 0, memberCount)
	profiles := make([]fixtureProfile, 0, memberCount)
	devices := make([]fixtureDevice, 0, memberCount)
	members := make([]fixtureMember, 0, memberCount)
	results := make([]fixtureMemberResult, 0, memberCount)
	for index := 0; index < memberCount; index++ {
		userID := uint64(snowflake.Generate())
		deviceID := uint64(snowflake.Generate())
		username := fmt.Sprintf("load_gc_%s_%d", runID, index)
		users = append(users, fixtureUser{
			ID:           userID,
			Username:     username,
			PasswordHash: passwordHash,
			Status:       1,
		})
		profiles = append(profiles, fixtureProfile{
			UserID:   userID,
			Nickname: fmt.Sprintf("容量用户%d", index+1),
		})
		devices = append(devices, fixtureDevice{
			ID:         deviceID,
			UserID:     userID,
			Platform:   1,
			DeviceName: fmt.Sprintf("capacity-device-%d", index+1),
		})
		members = append(members, fixtureMember{
			UserID: userID,
			Role:   1,
		})
		results = append(results, fixtureMemberResult{
			UserID:   strconv.FormatUint(userID, 10),
			DeviceID: strconv.FormatUint(deviceID, 10),
		})
	}

	conversationID := uint64(snowflake.Generate())
	ownerID := users[0].ID
	members[0].Role = 3
	for index := range members {
		members[index].ConversationID = conversationID
	}
	conversation := fixtureConversation{
		ID:               conversationID,
		ConversationType: 2,
		OwnerID:          ownerID,
		Name:             "RelayIM 容量测试群 " + runID,
		Status:           1,
	}

	err = db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&users).Error; err != nil {
			return fmt.Errorf("创建压测用户: %w", err)
		}
		if err := tx.Create(&profiles).Error; err != nil {
			return fmt.Errorf("创建压测用户资料: %w", err)
		}
		if err := tx.Create(&devices).Error; err != nil {
			return fmt.Errorf("创建压测设备: %w", err)
		}
		if err := tx.Create(&conversation).Error; err != nil {
			return fmt.Errorf("创建压测群会话: %w", err)
		}
		if err := tx.CreateInBatches(&members, 100).Error; err != nil {
			return fmt.Errorf("创建压测群成员: %w", err)
		}
		return nil
	})
	if err != nil {
		return fixtureResult{}, err
	}
	return fixtureResult{
		RunID:          runID,
		ConversationID: strconv.FormatUint(conversationID, 10),
		Members:        results,
	}, nil
}

// issueTokens 为每个压测用户签发写入 Redis 白名单的短期 Token
func issueTokens(
	ctx context.Context,
	members []fixtureMemberResult,
) error {
	for index := range members {
		userID, err := strconv.ParseUint(members[index].UserID, 10, 64)
		if err != nil {
			return fmt.Errorf("解析压测用户 ID: %w", err)
		}
		token, err := authn.DefaultManager.IssueToken(ctx, userID)
		if err != nil {
			return fmt.Errorf("签发第 %d 个压测 Token: %w", index+1, err)
		}
		members[index].Token = token
	}
	return nil
}

// writeFixture 以仅当前用户可读写的权限保存 Token 与测试标识
func writeFixture(path string, result fixtureResult) error {
	if directory := filepath.Dir(path); directory != "." {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			return fmt.Errorf("创建夹具输出目录: %w", err)
		}
	}
	body, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("序列化容量夹具: %w", err)
	}
	if err := os.WriteFile(path, body, 0o600); err != nil {
		return fmt.Errorf("写入容量夹具: %w", err)
	}
	return nil
}
