package identity

import (
	"context"
	"net"
	"os"
	"strconv"
	"testing"
	"time"

	"ginChatProject/internal/config"
	"ginChatProject/internal/platform/jwt"
	"ginChatProject/internal/platform/redisstore"
	"ginChatProject/internal/platform/snowflake"
)

func TestSystemTokenIssuerIntegration(t *testing.T) {
	redisAddr := os.Getenv("GINCHAT_TEST_REDIS_ADDR")
	if redisAddr == "" {
		t.Skip("未设置 GINCHAT_TEST_REDIS_ADDR，跳过 Redis 集成测试")
	}

	redisHost, redisPortText, err := net.SplitHostPort(redisAddr)
	if err != nil {
		t.Fatalf("测试 Redis 地址必须为 host:port: %v", err)
	}
	redisPort, err := strconv.Atoi(redisPortText)
	if err != nil {
		t.Fatalf("解析测试 Redis 端口失败: %v", err)
	}

	if err := redisstore.InitRedis(config.RedisConfig{
		Host:     redisHost,
		Port:     redisPort,
		Password: os.Getenv("GINCHAT_TEST_REDIS_PASSWORD"),
		DB:       0,
	}); err != nil {
		t.Fatalf("连接测试 Redis 失败: %v", err)
	}

	oldJWTConfig := jwt.JwtCfg
	jwt.JwtCfg = config.JWTConfig{
		Secret:      "integration-test-secret",
		ExpireHours: 1,
	}
	t.Cleanup(func() {
		jwt.JwtCfg = oldJWTConfig
	})

	if err := snowflake.Init(1); err != nil {
		t.Fatalf("初始化 Snowflake 失败: %v", err)
	}

	userID := uint64(snowflake.Generate())
	issuer := NewSystemTokenIssuer()
	token, err := issuer.IssueToken(context.Background(), userID)
	if err != nil {
		t.Fatalf("签发 Token 失败: %v", err)
	}

	t.Cleanup(func() {
		redisstore.RDB.Del(context.Background(), "token:"+token)
	})

	claims, err := jwt.ParseToken(token)
	if err != nil {
		t.Fatalf("解析 Token 失败: %v", err)
	}
	if claims.UserID != userID {
		t.Fatalf("Token 用户 ID = %d，期望 %d", claims.UserID, userID)
	}

	ttl, err := redisstore.RDB.TTL(context.Background(), "token:"+token).Result()
	if err != nil {
		t.Fatalf("读取 Token TTL 失败: %v", err)
	}
	if ttl <= 0 || ttl > time.Hour {
		t.Fatalf("Token TTL = %v，期望 0 到 1 小时之间", ttl)
	}
}
