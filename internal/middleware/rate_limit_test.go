package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/PYang176/RelayIM/internal/platform/ratelimit"
	"github.com/gin-gonic/gin"
)

type rateLimitCall struct {
	namespace    string
	subject      string
	subjectLimit int
	globalLimit  int
	window       time.Duration
}

type stubRateLimitChecker struct {
	decision ratelimit.Decision
	err      error
	calls    []rateLimitCall
}

// Allow 判断当前测试请求是否允许通过限流，并记录调用参数
func (s *stubRateLimitChecker) Allow(
	_ context.Context,
	namespace string,
	subject string,
	subjectLimit int,
	globalLimit int,
	window time.Duration,
) (ratelimit.Decision, error) {
	s.calls = append(s.calls, rateLimitCall{
		namespace:    namespace,
		subject:      subject,
		subjectLimit: subjectLimit,
		globalLimit:  globalLimit,
		window:       window,
	})
	return s.decision, s.err
}

// TestRateLimitByIPUsesClientIP 覆盖按 IP 限流使用真实客户端地址
func TestRateLimitByIPUsesClientIP(t *testing.T) {
	gin.SetMode(gin.TestMode)
	checker := &stubRateLimitChecker{
		decision: ratelimit.Decision{Allowed: true},
	}
	router := gin.New()
	router.POST(
		"/login",
		RateLimit(checker, middlewareTestPolicy(RateLimitByIP), nil),
		func(c *gin.Context) {
			c.Status(http.StatusNoContent)
		},
	)

	request := httptest.NewRequest(http.MethodPost, "/login", nil)
	request.RemoteAddr = "203.0.113.8:4321"
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("状态码 = %d，期望 204", response.Code)
	}
	if len(checker.calls) != 1 || checker.calls[0].subject != "203.0.113.8" {
		t.Fatalf("限流主体记录 = %+v", checker.calls)
	}
}

// TestRateLimitByUserRequiresAuthenticatedUser 覆盖按用户限流缺少认证身份时的拒绝
func TestRateLimitByUserRequiresAuthenticatedUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	checker := &stubRateLimitChecker{
		decision: ratelimit.Decision{Allowed: true},
	}
	router := gin.New()
	router.POST(
		"/messages",
		RateLimit(checker, middlewareTestPolicy(RateLimitByUser), nil),
		func(c *gin.Context) {
			c.Status(http.StatusNoContent)
		},
	)

	response := httptest.NewRecorder()
	router.ServeHTTP(
		response,
		httptest.NewRequest(http.MethodPost, "/messages", nil),
	)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("状态码 = %d，期望 401", response.Code)
	}
	if len(checker.calls) != 0 {
		t.Fatalf("未认证请求不应调用限流器，实际调用 %d 次", len(checker.calls))
	}
}

// TestRateLimitMapsSubjectScopeAndReturnsRetryAfter 覆盖限流主体、范围响应头和 Retry-After
func TestRateLimitMapsSubjectScopeAndReturnsRetryAfter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	checker := &stubRateLimitChecker{
		decision: ratelimit.Decision{
			Allowed:    false,
			RetryAfter: 750 * time.Millisecond,
			Scope:      "subject",
		},
	}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(ContextUserIDKey, uint64(42))
		c.Next()
	})
	router.POST(
		"/messages",
		RateLimit(checker, middlewareTestPolicy(RateLimitByUser), nil),
		func(c *gin.Context) {
			c.Status(http.StatusNoContent)
		},
	)

	response := httptest.NewRecorder()
	router.ServeHTTP(
		response,
		httptest.NewRequest(http.MethodPost, "/messages", nil),
	)

	if response.Code != http.StatusTooManyRequests {
		t.Fatalf("状态码 = %d，期望 429", response.Code)
	}
	if response.Header().Get("Retry-After") != "1" {
		t.Fatalf("Retry-After = %q", response.Header().Get("Retry-After"))
	}
	if response.Header().Get("X-RateLimit-Scope") != "user" {
		t.Fatalf(
			"X-RateLimit-Scope = %q",
			response.Header().Get("X-RateLimit-Scope"),
		)
	}
}

// TestRateLimitReturnsServiceUnavailableWhenCheckerFails 覆盖限流检查器故障时的 503 降级
func TestRateLimitReturnsServiceUnavailableWhenCheckerFails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	checker := &stubRateLimitChecker{err: errors.New("Redis 不可用")}
	router := gin.New()
	router.POST(
		"/login",
		RateLimit(checker, middlewareTestPolicy(RateLimitByIP), nil),
		func(c *gin.Context) {
			c.Status(http.StatusNoContent)
		},
	)

	response := httptest.NewRecorder()
	router.ServeHTTP(
		response,
		httptest.NewRequest(http.MethodPost, "/login", nil),
	)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("状态码 = %d，期望 503", response.Code)
	}
}

// middlewareTestPolicy 提供限流中间件测试使用的固定策略
func middlewareTestPolicy(subject RateLimitSubject) RateLimitPolicy {
	return RateLimitPolicy{
		Namespace:    "test",
		Subject:      subject,
		SubjectLimit: 2,
		GlobalLimit:  10,
		Window:       time.Second,
	}
}
