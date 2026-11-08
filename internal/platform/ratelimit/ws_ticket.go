// Package ratelimit 提供基于 Redis 的原子请求限流能力。
package ratelimit

import (
	"context"
	"fmt"
	"strconv"
	"sync/atomic"
	"time"

	"ginChatProject/internal/config"
	"github.com/redis/go-redis/v9"
)

const wsTicketRateLimitScript = `
local now = tonumber(ARGV[4])
local window = tonumber(ARGV[3])
local cutoff = now - window

redis.call("ZREMRANGEBYSCORE", KEYS[1], "-inf", cutoff)
redis.call("ZREMRANGEBYSCORE", KEYS[2], "-inf", cutoff)

local userCount = redis.call("ZCARD", KEYS[1])
local globalCount = redis.call("ZCARD", KEYS[2])
local userLimit = tonumber(ARGV[1])
local globalLimit = tonumber(ARGV[2])

if userCount >= userLimit then
    local oldest = redis.call("ZRANGE", KEYS[1], 0, 0, "WITHSCORES")
    local retryAfter = 1
    if #oldest >= 2 then
        retryAfter = math.max(1, tonumber(oldest[2]) + window - now)
    end
    return {0, userCount, globalCount, retryAfter, 1}
end

if globalCount >= globalLimit then
    local oldest = redis.call("ZRANGE", KEYS[2], 0, 0, "WITHSCORES")
    local retryAfter = 1
    if #oldest >= 2 then
        retryAfter = math.max(1, tonumber(oldest[2]) + window - now)
    end
    return {0, userCount, globalCount, retryAfter, 2}
end

redis.call("ZADD", KEYS[1], now, ARGV[5])
redis.call("ZADD", KEYS[2], now, ARGV[5])
redis.call("PEXPIRE", KEYS[1], window + 1000)
redis.call("PEXPIRE", KEYS[2], window + 1000)

return {1, userCount + 1, globalCount + 1, 0, 0}
`

var wsTicketRequestSequence atomic.Uint64

// Decision 是一次限流检查的结果。
type Decision struct {
	// Allowed 表示请求是否可以继续执行。
	Allowed bool

	// RetryAfter 是建议客户端等待的时间。
	RetryAfter time.Duration

	// Scope 是触发限制的范围：user 或 global。
	Scope string
}

// WSTicketLimiter 使用 Redis ZSET 原子检查全局和单用户滑动窗口配额。
type WSTicketLimiter struct {
	client       redis.Scripter
	script       *redis.Script
	keyPrefix    string
	userLimit    int
	globalLimit  int
	windowMillis int
	now          func() time.Time
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
		now:          time.Now,
	}
}

// Allow 检查当前用户与全局请求配额。
func (l *WSTicketLimiter) Allow(
	ctx context.Context,
	userID uint64,
) (Decision, error) {
	now := l.now().UnixMilli()
	userKey := fmt.Sprintf(
		"%s:ws_ticket:user:%d:sliding",
		l.keyPrefix,
		userID,
	)
	globalKey := fmt.Sprintf(
		"%s:ws_ticket:global:sliding",
		l.keyPrefix,
	)
	member := fmt.Sprintf(
		"%d:%d",
		now,
		wsTicketRequestSequence.Add(1),
	)
	values, err := l.script.Run(
		ctx,
		l.client,
		[]string{userKey, globalKey},
		l.userLimit,
		l.globalLimit,
		l.windowMillis,
		now,
		member,
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
