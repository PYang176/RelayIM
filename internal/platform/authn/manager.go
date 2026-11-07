// Package authn 统一管理 JWT 签发、校验和 Redis 会话撤销。
package authn

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
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

	// ErrWSTicketInvalid 表示一次性 WebSocket Ticket 无效、过期或已被使用。
	ErrWSTicketInvalid = errors.New("WebSocket Ticket 无效或已使用")
)

const wsTicketTTL = 30 * time.Second

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

// IssueWSTicket 生成短时、单次使用的 WebSocket 建连票据。
func (m *Manager) IssueWSTicket(
	ctx context.Context,
	userID uint64,
) (string, time.Duration, error) {
	if userID == 0 {
		return "", 0, ErrInvalidToken
	}

	randomBytes := make([]byte, 32)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", 0, fmt.Errorf("%w: 生成 WebSocket Ticket: %w", ErrAuthUnavailable, err)
	}
	ticket := base64.RawURLEncoding.EncodeToString(randomBytes)
	key := wsTicketKey(ticket)
	if err := redisstore.RDB.Set(ctx, key, userID, wsTicketTTL).Err(); err != nil {
		return "", 0, fmt.Errorf("%w: 保存 WebSocket Ticket: %w", ErrAuthUnavailable, err)
	}
	return ticket, wsTicketTTL, nil
}

// ConsumeWSTicket 原子读取并删除 Ticket，保证同一票据只能建立一次连接。
func (m *Manager) ConsumeWSTicket(
	ctx context.Context,
	ticket string,
) (uint64, error) {
	ticket = strings.TrimSpace(ticket)
	if ticket == "" {
		return 0, ErrWSTicketInvalid
	}

	value, err := redisstore.RDB.GetDel(ctx, wsTicketKey(ticket)).Result()
	if errors.Is(err, redis.Nil) || value == "" {
		return 0, ErrWSTicketInvalid
	}
	if err != nil {
		return 0, fmt.Errorf("%w: 读取 WebSocket Ticket: %w", ErrAuthUnavailable, err)
	}

	userID, err := strconv.ParseUint(value, 10, 64)
	if err != nil || userID == 0 {
		return 0, ErrWSTicketInvalid
	}
	return userID, nil
}

func wsTicketKey(ticket string) string {
	digest := sha256.Sum256([]byte(ticket))
	return fmt.Sprintf("ws_ticket:%x", digest)
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
