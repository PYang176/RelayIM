package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/PYang176/RelayIM/internal/platform/observability"
	"github.com/gin-gonic/gin"
)

const requestIDHeader = "X-Request-ID"

type requestIDContextKey struct{}

var fallbackRequestID atomic.Uint64

// ObserveRequest 为每个请求注入 Request ID，记录结构化访问日志和 Prometheus 指标。
func ObserveRequest(metrics *observability.Metrics) gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := normalizedRequestID(c.GetHeader(requestIDHeader))
		c.Header(requestIDHeader, requestID)
		c.Set("request_id", requestID)
		c.Request = c.Request.WithContext(context.WithValue(
			c.Request.Context(),
			requestIDContextKey{},
			requestID,
		))

		startedAt := time.Now()
		if metrics != nil {
			metrics.BeginRequest()
		}
		c.Next()

		route := c.FullPath()
		if route == "" {
			route = "unmatched"
		}
		status := c.Writer.Status()
		duration := time.Since(startedAt)
		if metrics != nil {
			metrics.ObserveHTTP(
				c.Request.Method,
				route,
				status,
				duration,
			)
		}
		if shouldLogAccess(c.Request.URL.Path) {
			slog.Info(
				"HTTP 请求完成",
				"request_id", requestID,
				"method", c.Request.Method,
				"route", route,
				"status", status,
				"duration_ms", duration.Milliseconds(),
			)
		}
	}
}

// RequestIDFromContext 返回当前 HTTP 请求的追踪 ID。
func RequestIDFromContext(ctx context.Context) string {
	value, _ := ctx.Value(requestIDContextKey{}).(string)
	return value
}

// normalizedRequestID 校验 Request ID 格式，非法值由服务端重新生成
func normalizedRequestID(value string) string {
	if len(value) >= 8 && len(value) <= 128 && safeRequestID(value) {
		return value
	}
	var randomBytes [16]byte
	if _, err := rand.Read(randomBytes[:]); err == nil {
		return hex.EncodeToString(randomBytes[:])
	}
	return fmt.Sprintf(
		"%s-%s",
		strconv.FormatInt(time.Now().UnixNano(), 36),
		strconv.FormatUint(fallbackRequestID.Add(1), 36),
	)
}

// safeRequestID 校验客户端 Request ID 是否只包含安全字符
func safeRequestID(value string) bool {
	for _, character := range value {
		switch {
		case character >= 'a' && character <= 'z':
		case character >= 'A' && character <= 'Z':
		case character >= '0' && character <= '9':
		case character == '-', character == '_', character == '.':
		default:
			return false
		}
	}
	return true
}

// shouldLogAccess 判断当前路径是否需要写入访问日志
func shouldLogAccess(path string) bool {
	switch path {
	case "/metrics", "/health/live", "/health/ready", "/ws":
		return false
	default:
		return true
	}
}
