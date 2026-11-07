package ws

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCheckOrigin(t *testing.T) {
	tests := []struct {
		name   string
		origin string
		want   bool
	}{
		{name: "非浏览器客户端", want: true},
		{name: "同源", origin: "http://api.example.com", want: true},
		{
			name:   "本地开发前端",
			origin: "http://localhost:5173",
			want:   true,
		},
		{
			name:   "本机回环地址",
			origin: "http://127.0.0.1:5173",
			want:   true,
		},
		{
			name:   "任意第三方来源",
			origin: "https://evil.example.com",
			want:   false,
		},
		{
			name:   "非法 Origin",
			origin: "://bad",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "http://api.example.com/ws", nil)
			if tt.origin != "" {
				request.Header.Set("Origin", tt.origin)
			}
			if got := checkOrigin(request); got != tt.want {
				t.Fatalf("checkOrigin() = %t，期望 %t", got, tt.want)
			}
		})
	}
}
