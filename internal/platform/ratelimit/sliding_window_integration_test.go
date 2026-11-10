package ratelimit

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// TestSlidingWindowLimiterIntegration 覆盖 Redis 滑动窗口限流的真实并发行为
func TestSlidingWindowLimiterIntegration(t *testing.T) {
	client := newRateLimitTestRedis(t)

	tests := []struct {
		name         string
		subjects     []string
		subjectLimit int
		globalLimit  int
		wantAllowed  []bool
		wantScope    string
	}{
		{
			name:         "单主体超过配额",
			subjects:     []string{"user-1", "user-1", "user-1"},
			subjectLimit: 2,
			globalLimit:  100,
			wantAllowed:  []bool{true, true, false},
			wantScope:    "subject",
		},
		{
			name:         "全局超过配额",
			subjects:     []string{"user-1", "user-2", "user-3"},
			subjectLimit: 10,
			globalLimit:  2,
			wantAllowed:  []bool{true, true, false},
			wantScope:    "global",
		},
	}

	for index, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prefix := fmt.Sprintf(
				"ratelimit_generic_%d_%d",
				time.Now().UnixNano(),
				index,
			)
			cleanupRateLimitKeys(t, client, prefix)
			limiter := NewSlidingWindowLimiter(client, prefix)
			now := time.Unix(1_800_000_000, 0)

			for requestIndex, subject := range tt.subjects {
				decision, err := limiter.allowAt(
					context.Background(),
					"message_send",
					subject,
					tt.subjectLimit,
					tt.globalLimit,
					time.Second,
					now,
				)
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

// TestSlidingWindowLimiterKeepsRecentRequestsAcrossFixedBoundaries 覆盖固定时间边界两侧仍保持滑动窗口
func TestSlidingWindowLimiterKeepsRecentRequestsAcrossFixedBoundaries(t *testing.T) {
	client := newRateLimitTestRedis(t)
	prefix := fmt.Sprintf("ratelimit_generic_boundary_%d", time.Now().UnixNano())
	cleanupRateLimitKeys(t, client, prefix)

	limiter := NewSlidingWindowLimiter(client, prefix)
	base := time.Unix(1_800_000_000, 0)

	for index, offset := range []time.Duration{
		899 * time.Millisecond,
		949 * time.Millisecond,
	} {
		decision, err := limiter.allowAt(
			context.Background(),
			"message_send",
			"user-1",
			2,
			100,
			time.Second,
			base.Add(offset),
		)
		if err != nil || !decision.Allowed {
			t.Fatalf(
				"窗口内第 %d 次请求 allowed=%v err=%v",
				index,
				decision.Allowed,
				err,
			)
		}
	}

	// 跨过固定窗口边界后，前两次请求仍在最近一秒内，不能重新获得完整配额。
	decision, err := limiter.allowAt(
		context.Background(),
		"message_send",
		"user-1",
		2,
		100,
		time.Second,
		base.Add(1050*time.Millisecond),
	)
	if err != nil {
		t.Fatalf("边界限流检查失败: %v", err)
	}
	if decision.Allowed || decision.Scope != "subject" {
		t.Fatalf("边界请求 decision=%+v，期望被单主体限制", decision)
	}
	if decision.RetryAfter < 800*time.Millisecond {
		t.Fatalf("RetryAfter=%v，期望接近 850ms", decision.RetryAfter)
	}

	decision, err = limiter.allowAt(
		context.Background(),
		"message_send",
		"user-1",
		2,
		100,
		time.Second,
		base.Add(1951*time.Millisecond),
	)
	if err != nil || !decision.Allowed {
		t.Fatalf("滑动窗口恢复失败: decision=%+v err=%v", decision, err)
	}
}

// TestSlidingWindowLimiterRejectsInvalidPolicy 覆盖非法限流策略的拒绝
func TestSlidingWindowLimiterRejectsInvalidPolicy(t *testing.T) {
	limiter := NewSlidingWindowLimiter(nil, "ginchat_test")
	_, err := limiter.Allow(
		context.Background(),
		"",
		"user-1",
		0,
		0,
		0,
	)
	if err == nil {
		t.Fatal("无效限流策略应返回错误")
	}
}

// newRateLimitTestRedis 创建速率限制TestRedis并完成必要依赖注入
func newRateLimitTestRedis(t *testing.T) *redis.Client {
	t.Helper()
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
	return client
}

// cleanupRateLimitKeys 清理限流集成测试创建的 Redis Key
func cleanupRateLimitKeys(
	t *testing.T,
	client *redis.Client,
	prefix string,
) {
	t.Helper()
	t.Cleanup(func() {
		keys, err := client.Keys(context.Background(), prefix+":*").Result()
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
}
