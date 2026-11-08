package middleware

import (
	"context"
	"math"
	"net/http"
	"strconv"

	"ginChatProject/internal/platform/observability"
	"ginChatProject/internal/platform/ratelimit"
	"github.com/gin-gonic/gin"
)

// WSTicketRateLimiter 是 Ticket 限流依赖的最小能力。
type WSTicketRateLimiter interface {
	Allow(ctx context.Context, userID uint64) (ratelimit.Decision, error)
}

// WSTicketRateLimit 在认证之后、设备创建之前限制 Ticket 请求速率。
func WSTicketRateLimit(
	limiter WSTicketRateLimiter,
	metrics *observability.Metrics,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		if limiter == nil {
			c.Next()
			return
		}
		userID, ok := CurrentUserID(c)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "登录用户信息无效",
			})
			return
		}

		decision, err := limiter.Allow(c.Request.Context(), userID)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{
				"error": "认证限流服务暂不可用",
			})
			return
		}
		if !decision.Allowed {
			retryAfter := int(math.Ceil(decision.RetryAfter.Seconds()))
			if retryAfter < 1 {
				retryAfter = 1
			}
			if metrics != nil {
				metrics.RecordRateLimitRejected(decision.Scope)
			}
			c.Header("Retry-After", strconv.Itoa(retryAfter))
			c.Header("X-RateLimit-Scope", decision.Scope)
			c.Header("Cache-Control", "no-store")
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error": "请求过于频繁，请稍后重试",
			})
			return
		}
		c.Next()
	}
}
