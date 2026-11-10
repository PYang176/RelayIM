package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"relayim/internal/platform/observability"
)

// TestTraceRequestPropagatesIncomingTrace 覆盖 HTTP 请求 Trace 上下文和响应头传播
func TestTraceRequestPropagatesIncomingTrace(t *testing.T) {
	gin.SetMode(gin.TestMode)
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)
	previousProvider := otel.GetTracerProvider()
	previousPropagator := otel.GetTextMapPropagator()
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() {
		otel.SetTracerProvider(previousProvider)
		otel.SetTextMapPropagator(previousPropagator)
		_ = provider.Shutdown(context.Background())
	})

	parentCtx, parentSpan := provider.Tracer("test").Start(
		context.Background(),
		"parent",
	)
	parentTraceID := observability.TraceID(parentCtx)
	parentSpan.End()

	router := gin.New()
	router.Use(TraceRequest())
	router.GET("/trace", func(c *gin.Context) {
		c.String(http.StatusOK, observability.TraceID(c.Request.Context()))
	})

	request := httptest.NewRequest(http.MethodGet, "/trace", nil)
	request.Header.Set("traceparent", observability.TraceParent(parentCtx))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("状态码 = %d", response.Code)
	}
	if response.Body.String() != parentTraceID {
		t.Fatalf(
			"处理函数 Trace ID = %q，期望复用 %q",
			response.Body.String(),
			parentTraceID,
		)
	}
	if response.Header().Get("X-Trace-ID") != parentTraceID {
		t.Fatalf(
			"X-Trace-ID = %q，期望 %q",
			response.Header().Get("X-Trace-ID"),
			parentTraceID,
		)
	}
}
