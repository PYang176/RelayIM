// Package ratelimit 提供基于 Redis 的原子请求限流能力。
package ratelimit

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"ginChatProject/internal/config"
	"github.com/redis/go-redis/v9"
)

const wsTicketRateLimitScript = `
local userCount = redis.call("INCR", KEYS[1])
if userCount == 1 then
    redis.call("PEXPIRE", KEYS[1], ARGV[3] + 1000)
end
if userCount > tonumber(ARGV[1]) then
    return {0, userCount, 0, redis.call("PTTL", KEYS[1]), 1}
end

local globalCount = redis.call("INCR", KEYS[2])
if globalCount == 1 then
    redis.call("PEXPIRE", KEYS[2], ARGV[3] + 1000)
end
if globalCount > tonumber(ARGV[2]) then
    return {0, userCount, globalCount, redis.call("PTTL", KEYS[2]), 2}
end

return {1, userCount, globalCount, 0, 0}
`

// Decision 是一次限流检查的结果。
type Decision struct {
	// Allowed 表示请求是否可以继续执行。
	Allowed bool

	// RetryAfter 是建议客户端等待的时间。
	RetryAfter time.Duration

	// Scope 是触发限制的范围：user 或 global。
	Scope string
}

// WSTicketLimiter 使用 Redis 原子检查全局和单用户固定窗口配额。
type WSTicketLimiter struct {
	client       redis.Scripter
	script       *redis.Script
	keyPrefix    string
	userLimit    int
	globalLimit  int
	windowMillis int
}

// NewWSTicketLimiter 创建 WebSocket Ticket 限流器。
func NewWSTicketLimiter(
	client redis.Scripter,
	cfg config.WSTicketRateLimitConfig,
	keyPrefix string,
) *WSTicketLimiter {
	return &WSTicketLimiter{
		client:       client,
		script:       redis.NewScript(wsTicketRateLimitScript),
		keyPrefix:    keyPrefix,
		userLimit:    cfg.UserLimit,
		globalLimit:  cfg.GlobalLimit,
		windowMillis: cfg.WindowMilliseconds,
	}
}

// Allow 检查当前用户与全局请求配额。
func (l *WSTicketLimiter) Allow(
	ctx context.Context,
	userID uint64,
) (Decision, error) {
	window := time.Now().UnixMilli() / int64(l.windowMillis)
	userKey := fmt.Sprintf(
		"%s:ws_ticket:user:%d:%d",
		l.keyPrefix,
		userID,
		window,
	)
	globalKey := fmt.Sprintf(
		"%s:ws_ticket:global:%d",
		l.keyPrefix,
		window,
	)
	values, err := l.script.Run(
		ctx,
		l.client,
		[]string{userKey, globalKey},
		l.userLimit,
		l.globalLimit,
		l.windowMillis,
	).Int64Slice()
	if err != nil {
		return Decision{}, fmt.Errorf("执行 WebSocket Ticket 限流: %w", err)
	}
	if len(values) != 5 {
		return Decision{}, fmt.Errorf(
			"WebSocket Ticket 限流返回值数量错误: %s",
			strconv.Itoa(len(values)),
		)
	}

	decision := Decision{
		Allowed: values[0] == 1,
	}
	if values[3] > 0 {
		decision.RetryAfter = time.Duration(values[3]) * time.Millisecond
	}
	switch values[4] {
	case 1:
		decision.Scope = "user"
	case 2:
		decision.Scope = "global"
	}
	return decision, nil
}
