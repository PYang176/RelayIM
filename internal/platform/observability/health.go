package observability

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

const readinessTimeout = 3 * time.Second

// DependencyCheck 是就绪检查中的单个外部依赖。
type DependencyCheck struct {
	// Name 是稳定且可展示的依赖名称。
	Name string

	// Check 执行一次带超时的依赖探测。
	Check func(ctx context.Context) error
}

// Health 提供进程存活和依赖就绪检查。
type Health struct {
	checks []DependencyCheck
}

// NewHealth 创建就绪检查集合。
func NewHealth(checks ...DependencyCheck) *Health {
	return &Health{checks: checks}
}

// LiveHandler 只表示当前进程仍能响应，不检查外部依赖。
func (h *Health) LiveHandler() http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writeHealthResponse(writer, http.StatusOK, map[string]any{
			"status": "alive",
		})
	})
}

// ReadyHandler 检查 MySQL、Redis 等启动必需依赖。
func (h *Health) ReadyHandler() http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		ctx, cancel := context.WithTimeout(
			request.Context(),
			readinessTimeout,
		)
		defer cancel()

		status := "ready"
		httpStatus := http.StatusOK
		checks := make(map[string]string, len(h.checks))
		for _, dependency := range h.checks {
			if err := dependency.Check(ctx); err != nil {
				checks[dependency.Name] = "down"
				status = "not_ready"
				httpStatus = http.StatusServiceUnavailable
				continue
			}
			checks[dependency.Name] = "up"
		}
		writeHealthResponse(writer, httpStatus, map[string]any{
			"status": status,
			"checks": checks,
		})
	})
}

// writeHealthResponse 把健康检查响应映射为 HTTP 响应
func writeHealthResponse(
	writer http.ResponseWriter,
	status int,
	payload map[string]any,
) {
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.WriteHeader(status)
	// 响应头和状态码已经提交，编码固定结构失败时没有二次修复通道。
	_ = json.NewEncoder(writer).Encode(payload)
}
