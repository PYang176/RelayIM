package objectstore

import (
	"context"
	"net/url"
	"testing"
	"time"

	"github.com/PYang176/RelayIM/internal/config"
)

// TestParsePublicEndpoint 覆盖浏览器访问地址的协议、主机和路径校验
func TestParsePublicEndpoint(t *testing.T) {
	tests := []struct {
		name       string
		raw        string
		wantHost   string
		wantSecure bool
		wantError  bool
	}{
		{
			name:     "未配置",
			raw:      "",
			wantHost: "",
		},
		{
			name:     "HTTP 地址",
			raw:      "http://localhost:8080",
			wantHost: "localhost:8080",
		},
		{
			name:       "HTTPS 地址",
			raw:        "https://media.example.com/",
			wantHost:   "media.example.com",
			wantSecure: true,
		},
		{
			name:      "不支持协议",
			raw:       "ftp://media.example.com",
			wantError: true,
		},
		{
			name:      "包含路径",
			raw:       "https://media.example.com/s3",
			wantError: true,
		},
		{
			name:      "包含查询参数",
			raw:       "https://media.example.com?token=test",
			wantError: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			host, secure, err := parsePublicEndpoint(test.raw)
			if (err != nil) != test.wantError {
				t.Fatalf("错误 = %v，期望错误=%t", err, test.wantError)
			}
			if host != test.wantHost || secure != test.wantSecure {
				t.Fatalf(
					"结果 = host:%q secure:%t，期望 host:%q secure:%t",
					host,
					secure,
					test.wantHost,
					test.wantSecure,
				)
			}
		})
	}
}

// TestPresignedGetURLUsesPublicEndpoint 验证预签名地址使用浏览器可访问的 Host
func TestPresignedGetURLUsesPublicEndpoint(t *testing.T) {
	presignClient, err := newPresignClient(config.ObjectStorageConfig{
		PublicEndpoint: "http://localhost:8333",
		AccessKey:      "test-access-key",
		SecretKey:      "test-secret-key",
		Region:         "us-east-1",
	})
	if err != nil {
		t.Fatalf("创建预签名客户端失败: %v", err)
	}
	client := &Client{
		client:        presignClient,
		presignClient: presignClient,
		bucket:        "relayim-media",
	}

	rawURL, err := client.PresignedGetURL(
		context.Background(),
		"users/1/attachments/2.txt",
		time.Minute,
	)
	if err != nil {
		t.Fatalf("生成预签名地址失败: %v", err)
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("解析预签名地址失败: %v", err)
	}
	if parsed.Scheme != "http" || parsed.Host != "localhost:8333" {
		t.Fatalf("预签名地址 = %s，期望 Host localhost:8333", parsed.Redacted())
	}
}
