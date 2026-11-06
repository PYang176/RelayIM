package middleware

import (
	"errors"
	"fmt"
	"ginChatProject/internal/platform/jwt"
	"ginChatProject/internal/platform/redisstore"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"net/http"
	"strings"
)

const (
	// ContextUserIDKey 保存不会因 32 位平台而降窄的 Snowflake 用户 ID。
	ContextUserIDKey = "user_id_u64"

	// ContextLegacyUserIDKey 保存旧 Handler 仍使用的 uint 用户 ID。
	ContextLegacyUserIDKey = "user_id"

	// ContextTokenKey 保存当前请求已验证的原始 Token。
	ContextTokenKey = "auth_token"

	// ContextJTIKey 保存 Token 的唯一标识，用于退出登录写入黑名单。
	ContextJTIKey = "auth_jti"

	// ContextExpiresAtKey 保存 Token 过期时间，用于计算黑名单 TTL。
	ContextExpiresAtKey = "auth_expires_at"
)

// AuthMiddleware JWT鉴权中间件
func AuthMiddleware() gin.HandlerFunc {
	return func(context *gin.Context) {
		// 从请求头中获取token
		tokenStr := authorizationToken(context.GetHeader("Authorization"))
		// 如果token为空，直接阻止接下来的请求
		if tokenStr == "" {
			context.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "未提供 token",
			})
			return
		}

		// token不为空，则解析token进行校验
		claims, err := jwt.ParseToken(tokenStr)
		if err != nil {
			context.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "token 无效或者已过期",
			})
			return
		}

		// 使用JTI作为Redis黑名单key
		blacklistKey := fmt.Sprintf("blacklist:%s", claims.JTI)

		// Redis 故障时返回 503，不能把认证基础设施不可用伪装成 Token 无效。
		exists, err := redisstore.RDB.Exists(context, blacklistKey).Result()
		if err != nil {
			context.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{
				"error": "认证服务暂不可用",
			})
			return
		}
		if exists > 0 {
			context.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "token已失效"})
			return
		}

		// 进行Redis校验
		key := fmt.Sprintf("token:%s", tokenStr)
		val, err := redisstore.RDB.Get(context, key).Result()
		if errors.Is(err, redis.Nil) || val == "" {
			context.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "token 已失效，请重新登录",
			})
			return
		}
		if err != nil {
			context.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{
				"error": "认证服务暂不可用",
			})
			return
		}

		//检验token携带的user_id是否与Redis缓存的一致
		if val != fmt.Sprintf("%d", claims.UserID) {
			context.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "token 不合法",
			})
			return
		}

		// 旧 Handler 仍使用 uint；新链路使用不会在 32 位平台降窄的 uint64。
		context.Set(ContextLegacyUserIDKey, uint(claims.UserID))
		context.Set(ContextUserIDKey, claims.UserID)
		context.Set(ContextTokenKey, tokenStr)
		context.Set(ContextJTIKey, claims.JTI)
		context.Set(ContextExpiresAtKey, claims.ExpiresAt.Time)
		context.Next()
	}
}

func authorizationToken(header string) string {
	header = strings.TrimSpace(header)
	parts := strings.Fields(header)
	if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
		return parts[1]
	}
	return header
}
