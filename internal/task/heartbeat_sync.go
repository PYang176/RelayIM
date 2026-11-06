package task

import (
	"context"
	"fmt"
	"ginChatProject/internal/platform/redisstore"
	"ginChatProject/internal/repository"
	"time"
)

type HeartbeatSyncTask struct {
	userRepo *repository.UserRepository
}

func NewHeartbeatSyncTask(userRepo *repository.UserRepository) *HeartbeatSyncTask {
	return &HeartbeatSyncTask{userRepo: userRepo}
}

// Start 启动定时同步任务
func (t *HeartbeatSyncTask) Start() {
	ticker := time.NewTicker(1 * time.Minute) // 每分钟同步一次
	defer ticker.Stop()

	for range ticker.C {
		if err := t.sync(); err != nil {
			fmt.Println("心跳同步失败:", err)
		}
	}
}

func (t *HeartbeatSyncTask) sync() error {
	ctx := context.Background()

	// 1. 从 Redis 获取所有心跳 key
	keys, err := redisstore.RDB.Keys(ctx, "heartbeat:*").Result()
	if err != nil {
		return err
	}

	// 2. 批量更新到数据库
	for _, key := range keys {
		// 解析 userID
		var userID uint64
		fmt.Sscanf(key, "heartbeat:%d", &userID)

		// 获取心跳时间戳
		ts, err := redisstore.RDB.Get(ctx, key).Int64()
		if err != nil {
			continue
		}

		// 更新数据库
		heartbeatTime := time.Unix(ts, 0)
		err = t.userRepo.UpdateHeartbeatTime(ctx, userID, heartbeatTime)
		if err != nil {
			fmt.Printf("更新用户 %d 心跳失败: %v\n", userID, err)
		}
	}

	return nil
}
