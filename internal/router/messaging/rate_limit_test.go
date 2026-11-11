package messaging

import (
	"testing"
	"time"

	"github.com/PYang176/RelayIM/internal/config"
	"github.com/PYang176/RelayIM/internal/middleware"
)

// TestMessageSendRateLimitPolicy 覆盖消息发送限流配置到策略的转换
func TestMessageSendRateLimitPolicy(t *testing.T) {
	policy := messageSendRateLimitPolicy(config.MessageRateLimitConfig{
		SubjectLimit:       30,
		GlobalLimit:        1200,
		WindowMilliseconds: 5000,
	})
	if policy.Subject != middleware.RateLimitByUser ||
		policy.SubjectLimit != 30 ||
		policy.GlobalLimit != 1200 ||
		policy.Window != 5*time.Second {
		t.Fatalf("消息限流策略转换错误: %+v", policy)
	}
}
