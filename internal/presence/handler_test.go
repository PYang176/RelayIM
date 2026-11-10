package presence

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"relayim/internal/middleware"
)

type stubQuerier struct {
	results []Status
	err     error
}

// Query 查询
func (s stubQuerier) Query(
	context.Context,
	[]uint64,
) ([]Status, error) {
	return s.results, s.err
}

// TestPresenceHandlerQuery 覆盖在线状态查询接口的响应转换
func TestPresenceHandlerQuery(t *testing.T) {
	tests := []struct {
		name       string
		userID     uint64
		body       string
		querier    stubQuerier
		wantStatus int
	}{
		{
			name:       "缺少认证用户",
			body:       `{"user_ids":["2"]}`,
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "用户 ID 非法",
			userID:     1,
			body:       `{"user_ids":["bad"]}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:   "查询成功",
			userID: 1,
			body:   `{"user_ids":["2"]}`,
			querier: stubQuerier{
				results: []Status{{UserID: 2, Online: true}},
			},
			wantStatus: http.StatusOK,
		},
		{
			name:       "Redis 不可用",
			userID:     1,
			body:       `{"user_ids":["2"]}`,
			querier:    stubQuerier{err: errors.New("redis unavailable")},
			wantStatus: http.StatusServiceUnavailable,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			router := gin.New()
			router.Use(func(c *gin.Context) {
				if tt.userID != 0 {
					c.Set(middleware.ContextUserIDKey, tt.userID)
				}
				c.Next()
			})
			router.POST("/api/v1/presence/query", NewHandler(tt.querier).Query)
			request := httptest.NewRequest(
				http.MethodPost,
				"/api/v1/presence/query",
				strings.NewReader(tt.body),
			)
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != tt.wantStatus {
				t.Fatalf(
					"状态码 = %d，期望 %d，响应 = %s",
					response.Code,
					tt.wantStatus,
					response.Body.String(),
				)
			}
		})
	}
}
