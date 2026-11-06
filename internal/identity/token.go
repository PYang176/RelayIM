package identity

import (
	"context"
	"fmt"
	"time"

	"ginChatProject/internal/platform/jwt"
	"ginChatProject/internal/platform/redisstore"
)

// TokenIssuer 抽象 Access Token 的签发和保存。
// Service 依赖该接口，测试可以使用不访问 Redis 的假实现。
type TokenIssuer interface {
	IssueToken(ctx context.Context, userID uint64) (string, error)
}

// SystemTokenIssuer 使用 JWT 和 Redis Token 白名单。
type SystemTokenIssuer struct{}

// NewSystemTokenIssuer 创建生产环境 Token 签发器。
func NewSystemTokenIssuer() *SystemTokenIssuer {
	return &SystemTokenIssuer{}
}

// IssueToken 生成 JWT，并以 Token 为键写入 Redis 白名单。
func (s *SystemTokenIssuer) IssueToken(
	ctx context.Context,
	userID uint64,
) (string, error) {
	expireHours := jwt.JwtCfg.ExpireHours
	if expireHours <= 0 {
		return "", fmt.Errorf("JWT 过期时间必须大于 0")
	}

	token, err := jwt.GenerateToken(userID, expireHours)
	if err != nil {
		return "", fmt.Errorf("生成 JWT 失败: %w", err)
	}

	// AuthMiddleware 先校验 JWT，再检查 Redis 中是否仍存在该 Token。
	// 删除 Redis 键即可立即撤销登录，不必等待 JWT 自然过期。
	key := "token:" + token
	if err := redisstore.RDB.Set(
		ctx,
		key,
		userID,
		time.Duration(expireHours)*time.Hour,
	).Err(); err != nil {
		return "", fmt.Errorf("保存 Token 白名单失败: %w", err)
	}
	return token, nil
}
