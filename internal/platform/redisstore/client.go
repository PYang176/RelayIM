package redisstore

import (
	"context"
	"fmt"
	"github.com/PYang176/RelayIM/internal/config"
	"github.com/redis/go-redis/v9"
)

var (
	Ctx = context.Background()
	RDB *redis.Client
)

// InitRedis 初始化 Redis 连接
func InitRedis(cfg config.RedisConfig) error {
	RDB = redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		Password: cfg.Password,
		DB:       cfg.DB,
	})

	if err := RDB.Ping(Ctx).Err(); err != nil {
		return fmt.Errorf("连接 Redis 失败: %w", err)
	}

	return nil
}

// Publish 发消息到redis
func Publish(ctx context.Context, channel string, msg string) error {
	return RDB.Publish(ctx, channel, msg).Err()
}

// Subscribe 订阅redis消息
func Subscribe(ctx context.Context, channel string) (string, error) {
	sub := RDB.PSubscribe(ctx, channel)
	defer sub.Close()
	msg, err := sub.ReceiveMessage(ctx)
	if err != nil {
		return "", err
	}
	return msg.Payload, err
}
