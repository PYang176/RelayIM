package middleware

import (
	"context"
	"net/http"

	"github.com/PYang176/RelayIM/internal/platform/observability"
	"github.com/PYang176/RelayIM/internal/platform/ratelimit"
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
			if metrics != nil {
				metrics.RecordRateLimitRejected(decision.Scope)
			}
			abortRateLimited(c, decision)
			return
		}
		c.Next()
	}
}
