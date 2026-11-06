package redisstore

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	GroupMsgStream        = "im:group:msg"       // 群消息流
	GroupMsgConsumerGroup = "im:group:consumers" // 消费者组
	PendingTimeout        = 30 * time.Second     // 待处理消息超时时间
)

// InitStream 初始化 Redis Stream 和消费者组
func InitStream(ctx context.Context) error {
	// 创建 Stream（如果不存在）
	err := RDB.XGroupCreateMkStream(ctx, GroupMsgStream, GroupMsgConsumerGroup, "$").Err()
	if err != nil && err.Error() != "BUSYGROUP Consumer Group name already exists" {
		return fmt.Errorf("创建消费者组失败: %v", err)
	}
	fmt.Println(" Redis Stream 初始化成功")
	return nil
}

// ProduceMessage 生产消息到 Stream
func ProduceMessage(ctx context.Context, stream string, values map[string]interface{}) (string, error) {
	args := &redis.XAddArgs{
		Stream: stream,
		Values: values,
	}
	return RDB.XAdd(ctx, args).Result()
}

// ConsumeMessages 消费消息
func ConsumeMessages(ctx context.Context, stream, group, consumer string, count int) ([]redis.XMessage, error) {
	// 读取新消息(以消费者组模式读取消息)
	streams, err := RDB.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    group,
		Consumer: consumer,
		Streams:  []string{stream, ">"}, //">"表示只读取从未被消费过的新消息
		Count:    int64(count),
		Block:    5 * time.Second, //如果没有消息，阻塞等待 5 秒，避免空转消耗 CPU
	}).Result()

	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if len(streams) == 0 {
		return nil, nil
	}

	return streams[0].Messages, nil
}

// AcknowledgeMessage 确认消息已处理
func AcknowledgeMessage(ctx context.Context, stream, group string, ids ...string) error {
	return RDB.XAck(ctx, stream, group, ids...).Err()
}

// GetPendingMessages 获取待处理消息（用于重试）
func GetPendingMessages(ctx context.Context, stream, group string, minIdleTime time.Duration) ([]redis.XMessage, error) {
	// 获取空闲超过指定时间的待处理消息
	pending, err := RDB.XPendingExt(ctx, &redis.XPendingExtArgs{
		Stream: stream,
		Group:  group,
		Start:  "-",
		End:    "+", //查询pending所有消息
		Count:  100, //每次最多查询 100 条
	}).Result()

	if err != nil {
		return nil, err
	}

	var pendingMsgs []redis.XMessage
	for _, p := range pending {
		// 空闲时间大于最小空闲时间
		if p.Idle > minIdleTime {
			// 获取具体的消息内容
			msgs, err := RDB.XRange(ctx, stream, p.ID, p.ID).Result()
			if err == nil && len(msgs) > 0 {
				pendingMsgs = append(pendingMsgs, redis.XMessage{
					ID:     p.ID,
					Values: msgs[0].Values,
				})
			}
		}
	}

	return pendingMsgs, nil
}
