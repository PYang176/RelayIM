package authn

import (
	"context"
	"errors"
	"net"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/PYang176/RelayIM/internal/config"
	"github.com/PYang176/RelayIM/internal/platform/jwt"
	"github.com/PYang176/RelayIM/internal/platform/redisstore"
	"github.com/PYang176/RelayIM/internal/platform/snowflake"
)

// TestManagerIntegration 覆盖认证管理器的真实 Redis 令牌生命周期
func TestManagerIntegration(t *testing.T) {
	redisAddr := os.Getenv("RELAYIM_TEST_REDIS_ADDR")
	if redisAddr == "" {
		t.Skip("未设置 RELAYIM_TEST_REDIS_ADDR，跳过 Redis 集成测试")
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
		Password: os.Getenv("RELAYIM_TEST_REDIS_PASSWORD"),
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

	manager := NewManager()
	userID := uint64(snowflake.Generate())
	token, err := manager.IssueToken(context.Background(), userID)
	if err != nil {
		t.Fatalf("签发 Token 失败: %v", err)
	}
	t.Cleanup(func() {
		redisstore.RDB.Del(context.Background(), "token:"+token)
	})

	session, err := manager.Authenticate(context.Background(), token)
	if err != nil {
		t.Fatalf("认证 Token 失败: %v", err)
	}
	if session.UserID != userID {
		t.Fatalf("会话用户 ID = %d，期望 %d", session.UserID, userID)
	}

	ttl, err := redisstore.RDB.TTL(context.Background(), "token:"+token).Result()
	if err != nil {
		t.Fatalf("读取 Token TTL 失败: %v", err)
	}
	if ttl <= 0 || ttl > time.Hour {
		t.Fatalf("Token TTL = %v，期望 0 到 1 小时之间", ttl)
	}

	if err := manager.Revoke(context.Background(), token, session.JTI, session.ExpiresAt); err != nil {
		t.Fatalf("撤销 Token 失败: %v", err)
	}
	if _, err := manager.Authenticate(context.Background(), token); !errors.Is(err, ErrTokenRevoked) {
		t.Fatalf("撤销后认证错误 = %v，期望 ErrTokenRevoked", err)
	}

	deviceID := uint64(9001)
	ticket, ttl, err := manager.IssueWSTicket(
		context.Background(),
		userID,
		deviceID,
	)
	if err != nil {
		t.Fatalf("签发 WebSocket Ticket 失败: %v", err)
	}
	if ttl <= 0 || ttl > 30*time.Second {
		t.Fatalf("WebSocket Ticket TTL = %v，期望 30 秒以内", ttl)
	}
	ticketSession, err := manager.ConsumeWSTicket(context.Background(), ticket)
	if err != nil {
		t.Fatalf("消费 WebSocket Ticket 失败: %v", err)
	}
	if ticketSession.UserID != userID || ticketSession.DeviceID != deviceID {
		t.Fatalf("Ticket 会话 = %+v，期望 user_id=%d device_id=%d",
			ticketSession,
			userID,
			deviceID,
		)
	}
	// 同一票据第二次使用时 Redis GETDEL 已删除记录，必须拒绝。
	if _, err := manager.ConsumeWSTicket(context.Background(), ticket); !errors.Is(err, ErrWSTicketInvalid) {
		t.Fatalf("重复消费 Ticket 错误 = %v，期望 ErrWSTicketInvalid", err)
	}
}
