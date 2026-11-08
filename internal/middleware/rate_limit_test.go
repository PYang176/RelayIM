package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"ginChatProject/internal/platform/ratelimit"
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

func middlewareTestPolicy(subject RateLimitSubject) RateLimitPolicy {
	return RateLimitPolicy{
		Namespace:    "test",
		Subject:      subject,
		SubjectLimit: 2,
		GlobalLimit:  10,
		Window:       time.Second,
	}
}
