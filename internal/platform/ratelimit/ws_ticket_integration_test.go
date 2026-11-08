package ratelimit

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"ginChatProject/internal/config"
	"github.com/redis/go-redis/v9"
)

func TestWSTicketLimiterIntegration(t *testing.T) {
	address := os.Getenv("GINCHAT_TEST_REDIS_ADDR")
	if address == "" {
		t.Skip("未设置 GINCHAT_TEST_REDIS_ADDR，跳过 Redis 限流集成测试")
	}
	client := redis.NewClient(&redis.Options{
		Addr:     address,
		Password: os.Getenv("GINCHAT_TEST_REDIS_PASSWORD"),
	})
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Errorf("关闭 Redis 客户端失败: %v", err)
		}
	})

	tests := []struct {
		name        string
		cfg         config.WSTicketRateLimitConfig
		userIDs     []uint64
		wantAllowed []bool
		wantScope   string
	}{
		{
			name: "单用户超过配额",
			cfg: config.WSTicketRateLimitConfig{
				GlobalLimit:        100,
				UserLimit:          2,
				WindowMilliseconds: 5000,
			},
			userIDs:     []uint64{1, 1, 1},
			wantAllowed: []bool{true, true, false},
			wantScope:   "user",
		},
		{
			name: "全局超过配额",
			cfg: config.WSTicketRateLimitConfig{
				GlobalLimit:        2,
				UserLimit:          10,
				WindowMilliseconds: 5000,
			},
			userIDs:     []uint64{1, 2, 3},
			wantAllowed: []bool{true, true, false},
			wantScope:   "global",
		},
	}

	for index, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prefix := fmt.Sprintf(
				"ratelimit_test_%d_%d",
				time.Now().UnixNano(),
				index,
			)
			t.Cleanup(func() {
				keys, err := client.Keys(
					context.Background(),
					prefix+":*",
				).Result()
				if err != nil {
					t.Errorf("查询限流测试键失败: %v", err)
					return
				}
				if len(keys) > 0 {
					if err := client.Del(context.Background(), keys...).Err(); err != nil {
						t.Errorf("清理限流测试键失败: %v", err)
					}
				}
			})

			limiter := NewWSTicketLimiter(client, tt.cfg, prefix)
			for requestIndex, userID := range tt.userIDs {
				decision, err := limiter.Allow(context.Background(), userID)
				if err != nil {
					t.Fatalf("第 %d 次限流检查失败: %v", requestIndex, err)
				}
				if decision.Allowed != tt.wantAllowed[requestIndex] {
					t.Fatalf(
						"第 %d 次 allowed=%v，期望 %v",
						requestIndex,
						decision.Allowed,
						tt.wantAllowed[requestIndex],
					)
				}
				if !decision.Allowed && decision.Scope != tt.wantScope {
					t.Fatalf("限流范围 = %q，期望 %q", decision.Scope, tt.wantScope)
				}
			}
		})
	}
}
