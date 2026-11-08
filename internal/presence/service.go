// Package presence 使用 Redis 维护多设备在线状态。
package presence

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	presenceTTL     = 70 * time.Second
	maxQueryUserIDs = 100
)

const queryPresenceScript = `
redis.call("ZREMRANGEBYSCORE", KEYS[1], "-inf", ARGV[1])
local count = redis.call("ZCARD", KEYS[1])
if count > 0 then
    redis.call("PEXPIRE", KEYS[1], ARGV[2])
end
return count
`

// Status 是一个用户的在线状态。
type Status struct {
	// UserID 是用户 Snowflake ID。
	UserID uint64

	// Online 表示最近 70 秒内至少有一台设备活跃。
	Online bool
}

// Service 负责设备心跳、断线清理和批量状态查询。
type Service struct {
	client *redis.Client
	prefix string
}

// NewService 创建在线状态服务。
func NewService(client *redis.Client, prefix string) *Service {
	return &Service{
		client: client,
		prefix: prefix,
	}
}

// Touch 更新指定用户设备的最近活跃时间。
func (s *Service) Touch(
	ctx context.Context,
	userID uint64,
	deviceID uint64,
) error {
	return s.touchAt(ctx, userID, deviceID, time.Now().UTC())
}

// Remove 删除已经关闭的 WebSocket 设备状态。
func (s *Service) Remove(
	ctx context.Context,
	userID uint64,
	deviceID uint64,
) error {
	if userID == 0 || deviceID == 0 {
		return fmt.Errorf("在线状态参数无效")
	}
	if err := s.client.ZRem(
		ctx,
		s.userKey(userID),
		strconv.FormatUint(deviceID, 10),
	).Err(); err != nil {
		return fmt.Errorf("删除设备在线状态: %w", err)
	}
	return nil
}

// Query 批量返回用户在线状态，最多查询 100 个去重用户。
func (s *Service) Query(
	ctx context.Context,
	userIDs []uint64,
) ([]Status, error) {
	return s.queryAt(ctx, userIDs, time.Now().UTC())
}

func (s *Service) touchAt(
	ctx context.Context,
	userID uint64,
	deviceID uint64,
	now time.Time,
) error {
	if userID == 0 || deviceID == 0 {
		return fmt.Errorf("在线状态参数无效")
	}
	key := s.userKey(userID)
	member := strconv.FormatUint(deviceID, 10)
	_, err := s.client.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.ZAdd(ctx, key, redis.Z{
			Score:  float64(now.UnixMilli()),
			Member: member,
		})
		pipe.PExpire(ctx, key, presenceTTL)
		return nil
	})
	if err != nil {
		return fmt.Errorf("更新设备在线状态: %w", err)
	}
	return nil
}

func (s *Service) queryAt(
	ctx context.Context,
	userIDs []uint64,
	now time.Time,
) ([]Status, error) {
	normalized, err := normalizeUserIDs(userIDs)
	if err != nil {
		return nil, err
	}

	pipe := s.client.Pipeline()
	commands := make([]*redis.Cmd, 0, len(normalized))
	cutoff := now.Add(-presenceTTL).UnixMilli()
	for _, userID := range normalized {
		commands = append(commands, pipe.Eval(
			ctx,
			queryPresenceScript,
			[]string{s.userKey(userID)},
			cutoff,
			presenceTTL.Milliseconds(),
		))
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return nil, fmt.Errorf("批量查询在线状态: %w", err)
	}

	results := make([]Status, 0, len(normalized))
	for index, userID := range normalized {
		count, err := commands[index].Int64()
		if err != nil {
			return nil, fmt.Errorf("解析用户 %d 在线状态: %w", userID, err)
		}
		results = append(results, Status{
			UserID: userID,
			Online: count > 0,
		})
	}
	return results, nil
}

func (s *Service) userKey(userID uint64) string {
	return fmt.Sprintf(
		"%s:presence:user:%d",
		s.prefix,
		userID,
	)
}

func normalizeUserIDs(userIDs []uint64) ([]uint64, error) {
	if len(userIDs) == 0 || len(userIDs) > maxQueryUserIDs {
		return nil, fmt.Errorf("在线状态查询数量必须为 1 到 %d", maxQueryUserIDs)
	}
	seen := make(map[uint64]struct{}, len(userIDs))
	normalized := make([]uint64, 0, len(userIDs))
	for _, userID := range userIDs {
		if userID == 0 {
			return nil, fmt.Errorf("在线状态查询包含无效用户")
		}
		if _, exists := seen[userID]; exists {
			continue
		}
		seen[userID] = struct{}{}
		normalized = append(normalized, userID)
	}
	return normalized, nil
}
