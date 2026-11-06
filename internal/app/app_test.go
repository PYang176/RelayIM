package app

import "testing"

func TestServerAddr(t *testing.T) {
	tests := []struct {
		name string
		env  string
		want string
	}{
		{
			name: "default",
			want: defaultServerAddr,
		},
		{
			name: "environment override",
			env:  "127.0.0.1:18081",
			want: "127.0.0.1:18081",
		},
		{
			name: "blank environment value",
			env:  "   ",
			want: defaultServerAddr,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("GINCHAT_SERVER_ADDR", tt.env)
			if got := serverAddr(); got != tt.want {
				t.Fatalf("serverAddr() = %q, want %q", got, tt.want)
			}
		})
	}
}
