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

			limiter := NewWSTicketLimiter(
				NewSlidingWindowLimiter(client, prefix),
				tt.cfg,
			)
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

func TestWSTicketLimiterSlidingWindowBoundary(t *testing.T) {
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

	prefix := fmt.Sprintf("ratelimit_sliding_%d", time.Now().UnixNano())
	t.Cleanup(func() {
		keys, err := client.Keys(context.Background(), prefix+":*").Result()
		if err != nil {
			t.Errorf("查询滑动窗口测试键失败: %v", err)
			return
		}
		if len(keys) > 0 {
			if err := client.Del(context.Background(), keys...).Err(); err != nil {
				t.Errorf("清理滑动窗口测试键失败: %v", err)
			}
		}
	})

	limiter := NewWSTicketLimiter(
		NewSlidingWindowLimiter(client, prefix),
		config.WSTicketRateLimitConfig{
			GlobalLimit:        100,
			UserLimit:          2,
			WindowMilliseconds: 1000,
		},
	)
	base := time.Unix(1_800_000_000, 0)
	current := base.Add(899 * time.Millisecond)
	limiter.now = func() time.Time {
		return current
	}

	for index := 0; index < 2; index += 1 {
		current = current.Add(50 * time.Millisecond)
		decision, err := limiter.Allow(context.Background(), 1)
		if err != nil || !decision.Allowed {
			t.Fatalf("窗口内第 %d 次请求 allowed=%v err=%v", index, decision.Allowed, err)
		}
	}

	// 跨过固定窗口边界后，旧的两次请求仍在最近一秒内，不能被立即放行。
	current = base.Add(1050 * time.Millisecond)
	decision, err := limiter.Allow(context.Background(), 1)
	if err != nil {
		t.Fatalf("边界限流检查失败: %v", err)
	}
	if decision.Allowed || decision.Scope != "user" {
		t.Fatalf("边界请求 decision=%+v，期望被单用户限制", decision)
	}
	if decision.RetryAfter < 800*time.Millisecond {
		t.Fatalf("RetryAfter=%v，期望接近 850ms", decision.RetryAfter)
	}

	// 最早请求移出滑动窗口后才能恢复。
	current = base.Add(1950 * time.Millisecond)
	decision, err = limiter.Allow(context.Background(), 1)
	if err != nil || !decision.Allowed {
		t.Fatalf("滑动窗口恢复失败: decision=%+v err=%v", decision, err)
	}
}
