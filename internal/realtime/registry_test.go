package realtime

import (
	"context"
	"fmt"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// TestRegistryIntegration 覆盖 Redis 实例路由的注册、查询和失效清理
func TestRegistryIntegration(t *testing.T) {
	address := os.Getenv("GINCHAT_TEST_REDIS_ADDR")
	if address == "" {
		t.Skip("未设置 GINCHAT_TEST_REDIS_ADDR，跳过 Redis 实时路由集成测试")
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

	prefix := fmt.Sprintf("realtime_route_test_%d", time.Now().UnixNano())
	t.Cleanup(func() {
		keys, err := client.Keys(context.Background(), prefix+":*").Result()
		if err != nil {
			t.Errorf("查询实时路由测试键失败: %v", err)
			return
		}
		if len(keys) > 0 {
			if err := client.Del(context.Background(), keys...).Err(); err != nil {
				t.Errorf("清理实时路由测试键失败: %v", err)
			}
		}
	})

	instanceA := NewRegistry(client, prefix, "instance-a")
	instanceB := NewRegistry(client, prefix, "instance-b")
	base := time.Now().UTC()
	if err := instanceA.touchAt(context.Background(), 1, 101, base); err != nil {
		t.Fatalf("写入实例 A 路由失败: %v", err)
	}
	if err := instanceB.touchAt(context.Background(), 1, 102, base); err != nil {
		t.Fatalf("写入实例 B 路由失败: %v", err)
	}
	routes, err := instanceA.queryInstancesAt(
		context.Background(),
		[]uint64{1, 2},
		base.Add(time.Second),
	)
	if err != nil {
		t.Fatalf("查询实时路由失败: %v", err)
	}
	if !slices.Equal(routes[1], []string{"instance-a", "instance-b"}) {
		t.Fatalf("用户 1 路由 = %v", routes[1])
	}
	if len(routes[2]) != 0 {
		t.Fatalf("离线用户不应有路由: %v", routes[2])
	}

	if err := instanceA.Remove(context.Background(), 1, 101); err != nil {
		t.Fatalf("删除实例 A 路由失败: %v", err)
	}
	routes, err = instanceB.queryInstancesAt(
		context.Background(),
		[]uint64{1},
		base.Add(2*time.Second),
	)
	if err != nil || !slices.Equal(routes[1], []string{"instance-b"}) {
		t.Fatalf("删除后路由 = %v, err=%v", routes[1], err)
	}

	if err := instanceB.touchAt(
		context.Background(),
		3,
		301,
		base.Add(-RouteTTL-time.Second),
	); err != nil {
		t.Fatalf("写入过期路由失败: %v", err)
	}
	routes, err = instanceB.queryInstancesAt(
		context.Background(),
		[]uint64{3},
		base,
	)
	if err != nil || len(routes[3]) != 0 {
		t.Fatalf("过期路由 = %v, err=%v", routes[3], err)
	}
}

// TestNormalizeRouteUserIDs 覆盖实时路由用户 ID 的规范化和去重
func TestNormalizeRouteUserIDs(t *testing.T) {
	got, err := normalizeRouteUserIDs([]uint64{2, 1, 2})
	if err != nil || !slices.Equal(got, []uint64{2, 1}) {
		t.Fatalf("normalizeRouteUserIDs() = %v, err=%v", got, err)
	}
	if _, err := normalizeRouteUserIDs(nil); err == nil {
		t.Fatal("空路由查询应返回错误")
	}
	if _, err := normalizeRouteUserIDs([]uint64{0}); err == nil {
		t.Fatal("零用户 ID 应返回错误")
	}
}
