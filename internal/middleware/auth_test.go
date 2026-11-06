package middleware

import "testing"

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
