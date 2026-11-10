package observability

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestHealthHandlers 覆盖存活和就绪检查的状态码与响应结构
func TestHealthHandlers(t *testing.T) {
	tests := []struct {
		name       string
		checks     []DependencyCheck
		wantStatus int
		wantState  string
	}{
		{
			name:       "无外部依赖时直接就绪",
			wantStatus: http.StatusOK,
			wantState:  "ready",
		},
		{
			name: "全部依赖正常",
			checks: []DependencyCheck{
				{
					Name: "mysql",
					Check: func(context.Context) error {
						return nil
					},
				},
			},
			wantStatus: http.StatusOK,
			wantState:  "ready",
		},
		{
			name: "关键依赖故障",
			checks: []DependencyCheck{
				{
					Name: "redis",
					Check: func(context.Context) error {
						return errors.New("不可用")
					},
				},
			},
			wantStatus: http.StatusServiceUnavailable,
			wantState:  "not_ready",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := NewHealth(tt.checks...).ReadyHandler()
			response := httptest.NewRecorder()
			handler.ServeHTTP(
				response,
				httptest.NewRequest(http.MethodGet, "/health/ready", nil),
			)
			if response.Code != tt.wantStatus {
				t.Fatalf("状态码 = %d，期望 %d", response.Code, tt.wantStatus)
			}
			var payload struct {
				Status string `json:"status"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
				t.Fatalf("解析响应失败: %v", err)
			}
			if payload.Status != tt.wantState {
				t.Fatalf("status = %q，期望 %q", payload.Status, tt.wantState)
			}
		})
	}
}

// TestLiveHandler 覆盖进程存活检查不依赖外部服务
func TestLiveHandler(t *testing.T) {
	response := httptest.NewRecorder()
	NewHealth().LiveHandler().ServeHTTP(
		response,
		httptest.NewRequest(http.MethodGet, "/health/live", nil),
	)
	if response.Code != http.StatusOK {
		t.Fatalf("状态码 = %d，期望 200", response.Code)
	}
}
