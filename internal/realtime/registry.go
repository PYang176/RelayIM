// Package realtime 提供跨实例 WebSocket 路由和节点级投递。
package realtime

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	// RouteTTL 是设备路由的活跃窗口。
	RouteTTL = 70 * time.Second

	maxRouteUserIDs = 500
)

const queryRoutesScript = `
redis.call("ZREMRANGEBYSCORE", KEYS[1], "-inf", ARGV[1])
local members = redis.call("ZRANGE", KEYS[1], 0, -1)
if #members > 0 then
    redis.call("PEXPIRE", KEYS[1], ARGV[2])
end
return members
`

// Registry 在 Redis 中维护用户设备当前连接的 Go 实例。
type Registry struct {
	client     *redis.Client
	prefix     string
	instanceID string
}

// NewRegistry 创建当前进程使用的跨实例路由注册表。
func NewRegistry(
	client *redis.Client,
	prefix string,
	instanceID string,
) *Registry {
	return &Registry{
		client:     client,
		prefix:     prefix,
		instanceID: instanceID,
	}
}

// Touch 更新当前实例上指定设备的路由活跃时间。
func (r *Registry) Touch(
	ctx context.Context,
	userID uint64,
	deviceID uint64,
) error {
	return r.touchAt(ctx, userID, deviceID, time.Now().UTC())
}

// Remove 删除已经关闭的 WebSocket 设备路由。
func (r *Registry) Remove(
	ctx context.Context,
	userID uint64,
	deviceID uint64,
) error {
	if userID == 0 || deviceID == 0 {
		return fmt.Errorf("实时路由参数无效")
	}
	if err := r.client.ZRem(
		ctx,
		r.userKey(userID),
		r.routeMember(deviceID),
	).Err(); err != nil {
		return fmt.Errorf("删除设备实时路由: %w", err)
	}
	return nil
}

// QueryInstances 批量返回用户当前连接所在的去重实例 ID。
func (r *Registry) QueryInstances(
	ctx context.Context,
	userIDs []uint64,
) (map[uint64][]string, error) {
	return r.queryInstancesAt(ctx, userIDs, time.Now().UTC())
}

func (r *Registry) touchAt(
	ctx context.Context,
	userID uint64,
	deviceID uint64,
	now time.Time,
) error {
	if userID == 0 || deviceID == 0 || strings.TrimSpace(r.instanceID) == "" {
		return fmt.Errorf("实时路由参数无效")
	}
	key := r.userKey(userID)
	_, err := r.client.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.ZAdd(ctx, key, redis.Z{
			Score:  float64(now.UnixMilli()),
			Member: r.routeMember(deviceID),
		})
		pipe.PExpire(ctx, key, RouteTTL)
		return nil
	})
	if err != nil {
		return fmt.Errorf("更新设备实时路由: %w", err)
	}
	return nil
}

func (r *Registry) queryInstancesAt(
	ctx context.Context,
	userIDs []uint64,
	now time.Time,
) (map[uint64][]string, error) {
	normalized, err := normalizeRouteUserIDs(userIDs)
	if err != nil {
		return nil, err
	}
	pipe := r.client.Pipeline()
	commands := make([]*redis.Cmd, 0, len(normalized))
	cutoff := now.Add(-RouteTTL).UnixMilli()
	for _, userID := range normalized {
		commands = append(commands, pipe.Eval(
			ctx,
			queryRoutesScript,
			[]string{r.userKey(userID)},
			cutoff,
			RouteTTL.Milliseconds(),
		))
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return nil, fmt.Errorf("批量查询实时路由: %w", err)
	}

	result := make(map[uint64][]string, len(normalized))
	for index, userID := range normalized {
		values, err := commands[index].StringSlice()
		if err != nil {
			return nil, fmt.Errorf("解析用户 %d 实时路由: %w", userID, err)
		}
		instances := make(map[string]struct{}, len(values))
		for _, value := range values {
			separator := strings.LastIndexByte(value, '|')
			if separator <= 0 {
				continue
			}
			instanceID := value[:separator]
			if instanceID != "" {
				instances[instanceID] = struct{}{}
			}
		}
		list := make([]string, 0, len(instances))
		for instanceID := range instances {
			list = append(list, instanceID)
		}
		sort.Strings(list)
		if len(list) > 0 {
			result[userID] = list
		}
	}
	return result, nil
}

func (r *Registry) userKey(userID uint64) string {
	return fmt.Sprintf("%s:ws:route:user:%d", r.prefix, userID)
}

func (r *Registry) routeMember(deviceID uint64) string {
	return r.instanceID + "|" + strconv.FormatUint(deviceID, 10)
}

func normalizeRouteUserIDs(userIDs []uint64) ([]uint64, error) {
	if len(userIDs) == 0 || len(userIDs) > maxRouteUserIDs {
		return nil, fmt.Errorf("实时路由查询数量必须为 1 到 %d", maxRouteUserIDs)
	}
	seen := make(map[uint64]struct{}, len(userIDs))
	normalized := make([]uint64, 0, len(userIDs))
	for _, userID := range userIDs {
		if userID == 0 {
			return nil, fmt.Errorf("实时路由查询包含无效用户")
		}
		if _, exists := seen[userID]; exists {
			continue
		}
		seen[userID] = struct{}{}
		normalized = append(normalized, userID)
	}
	return normalized, nil
}
