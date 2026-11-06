// Package authn 统一管理 JWT 签发、校验和 Redis 会话撤销。
package authn

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"ginChatProject/internal/platform/jwt"
	"ginChatProject/internal/platform/redisstore"
	"github.com/redis/go-redis/v9"
)

var (
	// ErrInvalidToken 表示 Token 格式、签名、主体或白名单内容不合法。
	ErrInvalidToken = errors.New("Token 无效")

	// ErrTokenRevoked 表示 Token 已退出登录或不在白名单中。
	ErrTokenRevoked = errors.New("Token 已失效")

	// ErrAuthUnavailable 表示 Redis 等认证依赖暂时不可用。
	ErrAuthUnavailable = errors.New("认证服务暂不可用")
)

// Session 是一次已通过校验的登录会话。
type Session struct {
	// UserID 是 Snowflake 用户 ID。
	UserID uint64

	// JTI 是 Token 唯一标识，用于退出登录时写入黑名单。
	JTI string

	// ExpiresAt 是 Token 过期时间，用于计算黑名单 TTL。
	ExpiresAt time.Time
}

// Manager 组合 JWT 与 Redis 白名单，HTTP 和 WebSocket 共用该组件。
type Manager struct{}

// NewManager 创建认证管理器。
func NewManager() *Manager {
	return &Manager{}
}

// DefaultManager 是应用级无状态认证管理器。
var DefaultManager = NewManager()

// IssueToken 生成 JWT 并写入 Redis 白名单。
func (m *Manager) IssueToken(ctx context.Context, userID uint64) (string, error) {
	expireHours := jwt.JwtCfg.ExpireHours
	if expireHours <= 0 {
		return "", fmt.Errorf("%w: JWT 过期时间必须大于 0", ErrAuthUnavailable)
	}

	token, err := jwt.GenerateToken(userID, expireHours)
	if err != nil {
		return "", fmt.Errorf("%w: 生成 JWT: %w", ErrAuthUnavailable, err)
	}

	key := "token:" + token
	if err := redisstore.RDB.Set(
		ctx,
		key,
		userID,
		time.Duration(expireHours)*time.Hour,
	).Err(); err != nil {
		return "", fmt.Errorf("%w: 保存 Token 白名单: %w", ErrAuthUnavailable, err)
	}
	return token, nil
}

// Authenticate 校验 JWT、黑名单和 Redis 白名单。
func (m *Manager) Authenticate(ctx context.Context, token string) (Session, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return Session{}, ErrInvalidToken
	}

	claims, err := jwt.ParseToken(token)
	if err != nil {
		return Session{}, fmt.Errorf("%w: %w", ErrInvalidToken, err)
	}

	blacklistKey := "blacklist:" + claims.JTI
	exists, err := redisstore.RDB.Exists(ctx, blacklistKey).Result()
	if err != nil {
		return Session{}, fmt.Errorf("%w: 查询黑名单: %w", ErrAuthUnavailable, err)
	}
	if exists > 0 {
		return Session{}, ErrTokenRevoked
	}

	tokenKey := "token:" + token
	storedUserID, err := redisstore.RDB.Get(ctx, tokenKey).Result()
	if errors.Is(err, redis.Nil) || storedUserID == "" {
		return Session{}, ErrTokenRevoked
	}
	if err != nil {
		return Session{}, fmt.Errorf("%w: 查询白名单: %w", ErrAuthUnavailable, err)
	}
	if storedUserID != strconv.FormatUint(claims.UserID, 10) {
		return Session{}, ErrInvalidToken
	}

	return Session{
		UserID:    claims.UserID,
		JTI:       claims.JTI,
		ExpiresAt: claims.ExpiresAt.Time,
	}, nil
}

// Revoke 将 Token 加入黑名单并删除白名单记录。
func (m *Manager) Revoke(
	ctx context.Context,
	token string,
	jti string,
	expiresAt time.Time,
) error {
	ttl := time.Until(expiresAt)
	if ttl > 0 {
		blacklistKey := "blacklist:" + jti
		if err := redisstore.RDB.Set(ctx, blacklistKey, "1", ttl).Err(); err != nil {
			return fmt.Errorf("%w: 写入黑名单: %w", ErrAuthUnavailable, err)
		}
	}

	if err := redisstore.RDB.Del(ctx, "token:"+token).Err(); err != nil {
		return fmt.Errorf("%w: 删除白名单: %w", ErrAuthUnavailable, err)
	}
	return nil
}
