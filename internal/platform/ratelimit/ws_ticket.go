package ratelimit

import (
	"context"
	"strconv"
	"time"

	"github.com/PYang176/RelayIM/internal/config"
)

// WSTicketLimiter 使用 Redis ZSET 原子检查全局和单用户滑动窗口配额。
type WSTicketLimiter struct {
	limiter      *SlidingWindowLimiter
	userLimit    int
	globalLimit  int
	windowMillis int
	now          func() time.Time
}

// NewWSTicketLimiter 创建 WebSocket Ticket 限流器。
func NewWSTicketLimiter(
	limiter *SlidingWindowLimiter,
	cfg config.WSTicketRateLimitConfig,
) *WSTicketLimiter {
	return &WSTicketLimiter{
		limiter:      limiter,
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
	decision, err := l.limiter.allowAt(
		ctx,
		"ws_ticket",
		strconv.FormatUint(userID, 10),
		l.userLimit,
		l.globalLimit,
		time.Duration(l.windowMillis)*time.Millisecond,
		l.now(),
	)
	if err != nil {
		return Decision{}, err
	}
	if decision.Scope == "subject" {
		decision.Scope = "user"
	}
	return decision, nil
}
