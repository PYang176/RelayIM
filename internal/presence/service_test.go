package presence

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// TestNormalizeUserIDs 覆盖在线状态批量查询用户 ID 的规范化
func TestNormalizeUserIDs(t *testing.T) {
	got, err := normalizeUserIDs([]uint64{2, 1, 2})
	if err != nil {
		t.Fatalf("normalizeUserIDs() 错误 = %v", err)
	}
	if len(got) != 2 || got[0] != 2 || got[1] != 1 {
		t.Fatalf("normalizeUserIDs() = %v", got)
	}
	if _, err := normalizeUserIDs(nil); err == nil {
		t.Fatal("空查询应返回错误")
	}
	if _, err := normalizeUserIDs([]uint64{0}); err == nil {
		t.Fatal("零用户 ID 应返回错误")
	}
}

// TestPresenceServiceIntegration 覆盖 Redis 多设备在线状态的写入、查询和清理
func TestPresenceServiceIntegration(t *testing.T) {
	address := os.Getenv("RELAYIM_TEST_REDIS_ADDR")
	if address == "" {
		t.Skip("未设置 RELAYIM_TEST_REDIS_ADDR，跳过 Redis 在线状态集成测试")
	}
	client := redis.NewClient(&redis.Options{
		Addr:     address,
		Password: os.Getenv("RELAYIM_TEST_REDIS_PASSWORD"),
	})
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Errorf("关闭 Redis 客户端失败: %v", err)
		}
	})

	prefix := fmt.Sprintf("presence_test_%d", time.Now().UnixNano())
	t.Cleanup(func() {
		keys, err := client.Keys(context.Background(), prefix+":*").Result()
		if err != nil {
			t.Errorf("查询在线状态测试键失败: %v", err)
			return
		}
		if len(keys) > 0 {
			if err := client.Del(context.Background(), keys...).Err(); err != nil {
				t.Errorf("清理在线状态测试键失败: %v", err)
			}
		}
	})

	service := NewService(client, prefix)
	base := time.Now().UTC()
	if err := service.touchAt(context.Background(), 1, 101, base); err != nil {
		t.Fatalf("写入设备 1 在线状态失败: %v", err)
	}
	if err := service.touchAt(context.Background(), 1, 102, base); err != nil {
		t.Fatalf("写入设备 2 在线状态失败: %v", err)
	}
	statuses, err := service.queryAt(
		context.Background(),
		[]uint64{1, 2},
		base.Add(time.Second),
	)
	if err != nil {
		t.Fatalf("查询在线状态失败: %v", err)
	}
	if len(statuses) != 2 || !statuses[0].Online || statuses[1].Online {
		t.Fatalf("在线状态 = %+v", statuses)
	}

	if err := service.Remove(context.Background(), 1, 101); err != nil {
		t.Fatalf("删除第一台设备失败: %v", err)
	}
	statuses, err = service.queryAt(
		context.Background(),
		[]uint64{1},
		base.Add(2*time.Second),
	)
	if err != nil || !statuses[0].Online {
		t.Fatalf("多设备在线状态 = %+v, err=%v", statuses, err)
	}
	if err := service.Remove(context.Background(), 1, 102); err != nil {
		t.Fatalf("删除第二台设备失败: %v", err)
	}
	statuses, err = service.queryAt(
		context.Background(),
		[]uint64{1},
		base.Add(3*time.Second),
	)
	if err != nil || statuses[0].Online {
		t.Fatalf("全部设备下线状态 = %+v, err=%v", statuses, err)
	}

	if err := service.touchAt(
		context.Background(),
		3,
		301,
		base.Add(-presenceTTL-time.Second),
	); err != nil {
		t.Fatalf("写入过期在线状态失败: %v", err)
	}
	statuses, err = service.queryAt(
		context.Background(),
		[]uint64{3},
		base,
	)
	if err != nil || statuses[0].Online {
		t.Fatalf("过期在线状态 = %+v, err=%v", statuses, err)
	}
}
