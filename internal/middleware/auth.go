package middleware

import (
	"errors"
	"ginChatProject/internal/platform/authn"
	"github.com/gin-gonic/gin"
	"net/http"
	"strings"
)

const (
	// ContextUserIDKey 保存不会因 32 位平台而降窄的 Snowflake 用户 ID。
	ContextUserIDKey = "user_id_u64"

	// ContextTokenKey 保存当前请求已验证的原始 Token。
	ContextTokenKey = "auth_token"

	// ContextJTIKey 保存 Token 的唯一标识，用于退出登录写入黑名单。
	ContextJTIKey = "auth_jti"

	// ContextExpiresAtKey 保存 Token 过期时间，用于计算黑名单 TTL。
	ContextExpiresAtKey = "auth_expires_at"

	// ContextSessionKey 保存认证组件返回的完整会话。
	ContextSessionKey = "auth_session"
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

		session, err := authn.DefaultManager.Authenticate(context, tokenStr)
		if err != nil {
			if errors.Is(err, authn.ErrAuthUnavailable) {
				context.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{
					"error": "认证服务暂不可用",
				})
				return
			}
			context.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "token 无效或者已过期",
			})
			return
		}

		context.Set(ContextUserIDKey, session.UserID)
		context.Set(ContextTokenKey, tokenStr)
		context.Set(ContextJTIKey, session.JTI)
		context.Set(ContextExpiresAtKey, session.ExpiresAt)
		context.Set(ContextSessionKey, session)
		context.Next()
	}
}

// CurrentUserID 返回认证中间件写入的 Snowflake 用户 ID。
func CurrentUserID(context *gin.Context) (uint64, bool) {
	value, exists := context.Get(ContextUserIDKey)
	if !exists {
		return 0, false
	}
	userID, ok := value.(uint64)
	if !ok || userID == 0 {
		return 0, false
	}
	return userID, true
}

// authorizationToken 从 Authorization 请求头提取 Bearer Token
func authorizationToken(header string) string {
	header = strings.TrimSpace(header)
	parts := strings.Fields(header)
	if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
		return parts[1]
	}
	return ""
}
