package middleware

import (
	"context"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"ginChatProject/internal/platform/observability"
	"ginChatProject/internal/platform/ratelimit"
	"github.com/gin-gonic/gin"
)

// RateLimitSubject 表示接口限流使用的请求主体。
type RateLimitSubject string

const (
	// RateLimitByUser 使用认证后的用户 ID，适合有状态写接口。
	RateLimitByUser RateLimitSubject = "user"

	// RateLimitByIP 使用客户端 IP，适合登录前接口。
	RateLimitByIP RateLimitSubject = "ip"
)

// RateLimitPolicy 描述一个接口的主体配额、全局配额和统计窗口。
type RateLimitPolicy struct {
	// Namespace 是稳定的业务命名空间，用于隔离不同接口的 Redis Key。
	Namespace string

	// Subject 表示按用户或 IP 限流。
	Subject RateLimitSubject

	// SubjectLimit 是单个主体在窗口内允许的请求数。
	SubjectLimit int

	// GlobalLimit 是所有主体共享的窗口请求上限。
	GlobalLimit int

	// Window 是滑动窗口长度。
	Window time.Duration
}

// RateLimitChecker 是通用限流中间件依赖的最小能力。
type RateLimitChecker interface {
	Allow(
		ctx context.Context,
		namespace string,
		subject string,
		subjectLimit int,
		globalLimit int,
		window time.Duration,
	) (ratelimit.Decision, error)
}

// RateLimit 按策略检查请求配额，超额返回 429，Redis 故障返回 503。
func RateLimit(
	checker RateLimitChecker,
	policy RateLimitPolicy,
	metrics *observability.Metrics,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		if checker == nil {
			c.Next()
			return
		}

		subject, ok := rateLimitRequestSubject(c, policy.Subject)
		if !ok {
			return
		}
		decision, err := checker.Allow(
			c.Request.Context(),
			policy.Namespace,
			subject,
			policy.SubjectLimit,
			policy.GlobalLimit,
			policy.Window,
		)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{
				"error": "限流服务暂不可用",
			})
			return
		}
		if decision.Allowed {
			c.Next()
			return
		}

		scope := decision.Scope
		if scope == "subject" {
			scope = string(policy.Subject)
		}
		if metrics != nil {
			metrics.RecordRateLimitRejected(scope)
		}
		decision.Scope = scope
		abortRateLimited(c, decision)
	}
}

// rateLimitRequestSubject 从认证上下文提取限流主体和范围
func rateLimitRequestSubject(
	c *gin.Context,
	subjectType RateLimitSubject,
) (string, bool) {
	switch subjectType {
	case RateLimitByUser:
		userID, ok := CurrentUserID(c)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "登录用户信息无效",
			})
			return "", false
		}
		return strconv.FormatUint(userID, 10), true
	case RateLimitByIP:
		clientIP := strings.TrimSpace(c.ClientIP())
		if clientIP == "" {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"error": "无法识别客户端地址",
			})
			return "", false
		}
		return clientIP, true
	default:
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
			"error": "限流策略配置错误",
		})
		return "", false
	}
}

// abortRateLimited 中断被限流请求，并写入 Retry-After 与限流范围响应头
func abortRateLimited(c *gin.Context, decision ratelimit.Decision) {
	retryAfter := int(math.Ceil(decision.RetryAfter.Seconds()))
	if retryAfter < 1 {
		retryAfter = 1
	}
	c.Header("Retry-After", strconv.Itoa(retryAfter))
	c.Header("X-RateLimit-Scope", decision.Scope)
	c.Header("Cache-Control", "no-store")
	c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
		"error": "请求过于频繁，请稍后重试",
	})
}
