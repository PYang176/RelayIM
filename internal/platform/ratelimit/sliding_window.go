// Package ratelimit 提供基于 Redis 的原子请求限流能力。
package ratelimit

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
)

const slidingWindowScript = `
local now = tonumber(ARGV[4])
local window = tonumber(ARGV[3])
local cutoff = now - window

redis.call("ZREMRANGEBYSCORE", KEYS[1], "-inf", cutoff)
redis.call("ZREMRANGEBYSCORE", KEYS[2], "-inf", cutoff)

local subjectCount = redis.call("ZCARD", KEYS[1])
local globalCount = redis.call("ZCARD", KEYS[2])
local subjectLimit = tonumber(ARGV[1])
local globalLimit = tonumber(ARGV[2])

if subjectCount >= subjectLimit then
    local oldest = redis.call("ZRANGE", KEYS[1], 0, 0, "WITHSCORES")
    local retryAfter = 1
    if #oldest >= 2 then
        retryAfter = math.max(1, tonumber(oldest[2]) + window - now)
    end
    return {0, subjectCount, globalCount, retryAfter, 1}
end

if globalCount >= globalLimit then
    local oldest = redis.call("ZRANGE", KEYS[2], 0, 0, "WITHSCORES")
    local retryAfter = 1
    if #oldest >= 2 then
        retryAfter = math.max(1, tonumber(oldest[2]) + window - now)
    end
    return {0, subjectCount, globalCount, retryAfter, 2}
end

redis.call("ZADD", KEYS[1], now, ARGV[5])
redis.call("ZADD", KEYS[2], now, ARGV[5])
redis.call("PEXPIRE", KEYS[1], window + 1000)
redis.call("PEXPIRE", KEYS[2], window + 1000)

return {1, subjectCount + 1, globalCount + 1, 0, 0}
`

var slidingWindowSequence atomic.Uint64

// Decision 是一次限流检查的结果。
type Decision struct {
	// Allowed 表示请求是否可以继续执行。
	Allowed bool

	// RetryAfter 是建议客户端等待的时间。
	RetryAfter time.Duration

	// Scope 是触发限制的范围：subject 或 global。
	Scope string
}

// SlidingWindowLimiter 使用 Redis ZSET 实现精确滑动窗口日志限流。
type SlidingWindowLimiter struct {
	client     redis.Scripter
	script     *redis.Script
	prefix     string
	instanceID string
}

// NewSlidingWindowLimiter 创建通用滑动窗口限流器。
func NewSlidingWindowLimiter(
	client redis.Scripter,
	prefix string,
) *SlidingWindowLimiter {
	return &SlidingWindowLimiter{
		client:     client,
		script:     redis.NewScript(slidingWindowScript),
		prefix:     prefix,
		instanceID: rateLimitInstanceID(),
	}
}

// Allow 使用当前时间检查一个业务命名空间内的主体和全局配额。
func (l *SlidingWindowLimiter) Allow(
	ctx context.Context,
	namespace string,
	subject string,
	subjectLimit int,
	globalLimit int,
	window time.Duration,
) (Decision, error) {
	return l.allowAt(
		ctx,
		namespace,
		subject,
		subjectLimit,
		globalLimit,
		window,
		time.Now(),
	)
}

// allowAt 在指定时间点判断测试限流策略是否放行
func (l *SlidingWindowLimiter) allowAt(
	ctx context.Context,
	namespace string,
	subject string,
	subjectLimit int,
	globalLimit int,
	window time.Duration,
	now time.Time,
) (Decision, error) {
	// 无效策略通常来自接线错误。提前拒绝可以避免写出永不失效或
	// 无上限的 Redis Key。
	if namespace == "" ||
		subject == "" ||
		subjectLimit <= 0 ||
		globalLimit <= 0 ||
		window <= 0 {
		return Decision{}, fmt.Errorf("滑动窗口限流参数无效")
	}

	nowMillis := now.UnixMilli()
	windowMillis := window.Milliseconds()
	subjectKey := fmt.Sprintf(
		"%s:rate:%s:subject:%s:sliding",
		l.prefix,
		namespace,
		subject,
	)
	globalKey := fmt.Sprintf(
		"%s:rate:%s:global:sliding",
		l.prefix,
		namespace,
	)
	member := fmt.Sprintf(
		"%s:%d:%d",
		l.instanceID,
		nowMillis,
		slidingWindowSequence.Add(1),
	)
	values, err := l.script.Run(
		ctx,
		l.client,
		[]string{subjectKey, globalKey},
		subjectLimit,
		globalLimit,
		windowMillis,
		nowMillis,
		member,
	).Int64Slice()
	if err != nil {
		return Decision{}, fmt.Errorf("执行滑动窗口限流: %w", err)
	}
	if len(values) != 5 {
		return Decision{}, fmt.Errorf(
			"滑动窗口限流返回值数量错误: %s",
			strconv.Itoa(len(values)),
		)
	}
	decision := Decision{Allowed: values[0] == 1}
	if values[3] > 0 {
		decision.RetryAfter = time.Duration(values[3]) * time.Millisecond
	}
	switch values[4] {
	case 1:
		decision.Scope = "subject"
	case 2:
		decision.Scope = "global"
	}
	return decision, nil
}

// rateLimitInstanceID 生成带唯一后缀的限流测试实例标识
func rateLimitInstanceID() string {
	// 多实例可能在同一毫秒处理同一用户的请求。成员值加入进程随机标识，
	// 避免 ZSET member 冲突导致少记一次请求、意外放宽配额。
	var random [8]byte
	if _, err := rand.Read(random[:]); err == nil {
		return hex.EncodeToString(random[:])
	}
	return strconv.FormatInt(time.Now().UnixNano(), 36)
}
