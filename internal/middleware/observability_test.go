package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"ginChatProject/internal/platform/observability"
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
