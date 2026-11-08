package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"ginChatProject/internal/platform/observability"
	"ginChatProject/internal/platform/ratelimit"
	"github.com/gin-gonic/gin"
)

func TestObserveRequestPreservesSafeRequestID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	metrics := observability.NewMetrics()
	router := gin.New()
	router.Use(ObserveRequest(metrics))
	router.GET("/test", func(c *gin.Context) {
		if got := RequestIDFromContext(c.Request.Context()); got != "client-request-123" {
			t.Fatalf("上下文请求 ID = %q", got)
		}
		c.Status(http.StatusNoContent)
	})

	request := httptest.NewRequest(http.MethodGet, "/test", nil)
	request.Header.Set(requestIDHeader, "client-request-123")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if got := response.Header().Get(requestIDHeader); got != "client-request-123" {
		t.Fatalf("响应请求 ID = %q", got)
	}
}

func TestObserveRequestReplacesUnsafeRequestID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(ObserveRequest(nil))
	router.GET("/test", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	request := httptest.NewRequest(http.MethodGet, "/test", nil)
	request.Header.Set(requestIDHeader, "bad request id\ninjected")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if got := response.Header().Get(requestIDHeader); got == "" ||
		got == "bad request id\ninjected" {
		t.Fatalf("不安全的请求 ID 未被替换: %q", got)
	}
}

type stubWSTicketLimiter struct {
	decision ratelimit.Decision
	err      error
}

func (s stubWSTicketLimiter) Allow(
	context.Context,
	uint64,
) (ratelimit.Decision, error) {
	return s.decision, s.err
}

func TestWSTicketRateLimitReturns429(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(ContextUserIDKey, uint64(42))
		c.Next()
	})
	router.POST(
		"/ws-ticket",
		WSTicketRateLimit(stubWSTicketLimiter{
			decision: ratelimit.Decision{
				Allowed:    false,
				RetryAfter: 750 * time.Millisecond,
				Scope:      "user",
			},
		}, nil),
		func(c *gin.Context) {
			c.Status(http.StatusNoContent)
		},
	)

	response := httptest.NewRecorder()
	router.ServeHTTP(
		response,
		httptest.NewRequest(http.MethodPost, "/ws-ticket", nil),
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
