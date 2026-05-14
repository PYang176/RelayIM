package redis_util

import (
	"context"
	"fmt"
	"ginChatProject/internal/config"
	"github.com/redis/go-redis/v9"
)

var (
	Ctx = context.Background()
	RDB *redis.Client
)

// InitRedis 初始化 Redis 连接
func InitRedis(cfg config.RedisConfig) {
	RDB = redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		Password: cfg.Password,
		DB:       cfg.DB,
	})

	if err := RDB.Ping(Ctx).Err(); err != nil {
		panic(fmt.Sprintf("redis connect failed: %v", err))
	}

	fmt.Println("redis connected")
}

// Publish 发消息到redis
func Publish(ctx context.Context, channel string, msg string) error {
	fmt.Println("publish...", msg)
	return RDB.Publish(ctx, channel, msg).Err()
}

// Subscribe 订阅redis消息
func Subscribe(ctx context.Context, channel string) (string, error) {
	sub := RDB.PSubscribe(ctx, channel)
	msg, err := sub.ReceiveMessage(ctx)
	fmt.Println("subscribe...", msg)
	return msg.Payload, err
}
