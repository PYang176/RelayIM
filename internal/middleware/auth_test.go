package middleware

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestAuthorizationToken(t *testing.T) {
	tests := []struct {
		name   string
		header string
		want   string
	}{
		{
			name:   "兼容旧原始 Token",
			header: "legacy-token",
			want:   "legacy-token",
		},
		{
			name:   "标准 Bearer Token",
			header: "Bearer jwt-token",
			want:   "jwt-token",
		},
		{
			name:   "大小写不敏感",
			header: "bearer jwt-token",
			want:   "jwt-token",
		},
		{
			name:   "空 Header",
			header: "   ",
			want:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := authorizationToken(tt.header); got != tt.want {
				t.Fatalf("authorizationToken(%q) = %q，期望 %q", tt.header, got, tt.want)
			}
		})
	}
}

func TestCurrentUserID(t *testing.T) {
	tests := []struct {
		name      string
		value     any
		set       bool
		wantID    uint64
		wantValid bool
	}{
		{
			name:      "有效用户",
			value:     uint64(42),
			set:       true,
			wantID:    42,
			wantValid: true,
		},
		{
			name:      "未写入上下文",
			wantValid: false,
		},
		{
			name:      "类型错误",
			value:     uint(42),
			set:       true,
			wantValid: false,
		},
		{
			name:      "用户 ID 为零",
			value:     uint64(0),
			set:       true,
			wantValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			context, _ := gin.CreateTestContext(httptest.NewRecorder())
			if tt.set {
				context.Set(ContextUserIDKey, tt.value)
			}

			userID, valid := CurrentUserID(context)
			if userID != tt.wantID || valid != tt.wantValid {
				t.Fatalf("CurrentUserID() = (%d, %t)，期望 (%d, %t)",
					userID,
					valid,
					tt.wantID,
					tt.wantValid,
				)
			}
		})
	}
}
