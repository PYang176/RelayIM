package consumer

import (
	"context"
	"encoding/json"
	"fmt"
	"ginChatProject/internal/model"
	"ginChatProject/internal/repository"
	"ginChatProject/internal/ws"
	"ginChatProject/utils/redis_util"
	"github.com/redis/go-redis/v9"
	"time"
)

type GroupMsgConsumer struct {
	msgRepo *repository.MessageRepository
}

func NewGroupMsgConsumer(msgRepo *repository.MessageRepository) *GroupMsgConsumer {
	return &GroupMsgConsumer{msgRepo: msgRepo}
}

// Start 启动消费者
func (c *GroupMsgConsumer) Start() {
	ctx := context.Background()

	// 初始化 Redis Stream
	if err := redis_util.InitStream(ctx); err != nil {
		panic(fmt.Sprintf("初始化 Redis Stream 失败: %v", err))
	}

	// 启动多个消费者实例
	for i := 0; i < 3; i++ {
		go c.consume(i)
	}

	// 启动重试机制
	go c.retryFailedMessages()
}

// consume 消费消息
func (c *GroupMsgConsumer) consume(workerID int) {
	ctx := context.Background()
	consumerName := fmt.Sprintf("worker-%d", workerID)

	fmt.Printf("🚀 消费者 %s 启动\n", consumerName)

	for {
		// 读取消息
		messages, err := redis_util.ConsumeMessages(
			ctx,
			redis_util.GroupMsgStream,
			redis_util.GroupMsgConsumerGroup,
			consumerName,
			10, // 每次最多处理10条
		)

		if err != nil {
			fmt.Printf("消费者 %s 读取消息失败: %v\n", consumerName, err)
			time.Sleep(1 * time.Second)
			continue
		}

		if len(messages) == 0 {
			continue
		}

		// 处理消息
		for _, msg := range messages {
			c.processMessage(ctx, msg, consumerName)
		}
	}
}

// processMessage 处理单条消息
func (c *GroupMsgConsumer) processMessage(ctx context.Context, msg redis.XMessage, consumerName string) {
	// 1. 解析消息
	messageID := msg.Values["message_id"].(string)
	payload := msg.Values["payload"].(string)

	var message model.Message
	if err := json.Unmarshal([]byte(payload), &message); err != nil {
		fmt.Printf("消息解析失败: %v\n", err)
		return
	}

	// 2. 获取群成员
	members, err := c.getGroupMembers(ctx, message.ReceiverID)
	if err != nil {
		fmt.Printf("获取群成员失败: %v\n", err)
		return
	}

	// 3. 推送给在线成员
	successCount := 0
	for _, member := range members {
		if member.UserID == message.SenderID {
			continue
		}

		if ws.Manager.IsOnline(uint(member.UserID)) {
			pushMsg := ws.WsMessage{
				Action:     ws.ActionMessage,
				MessageID:  message.MessageID,
				SenderID:   message.SenderID,
				ReceiverID: message.ReceiverID,
				MsgType:    int(message.MsgType),
				Content:    message.Content,
				URL:        message.URL,
			}

			data, _ := json.Marshal(pushMsg)
			if ws.Manager.Send(uint(member.UserID), data) {
				successCount++
			}
		}
	}

	// 4. 更新消息状态
	if successCount > 0 {
		c.msgRepo.UpdateStatus(ctx, messageID, model.MsgDelivered)
	}

	// 5. 确认消息已处理
	if err := redis_util.AcknowledgeMessage(
		ctx,
		redis_util.GroupMsgStream,
		redis_util.GroupMsgConsumerGroup,
		msg.ID,
	); err != nil {
		fmt.Printf("确认消息失败: %v\n", err)
	}

	fmt.Printf("✅ 消费者 %s 处理消息 %s 完成，推送 %d 人\n",
		consumerName, messageID, successCount)
}

// retryFailedMessages 重试失败的消息
func (c *GroupMsgConsumer) retryFailedMessages() {
	ctx := context.Background()

	for {
		time.Sleep(30 * time.Second) // 每30秒重试一次

		pendingMsgs, err := redis_util.GetPendingMessages(
			ctx,
			redis_util.GroupMsgStream,
			redis_util.GroupMsgConsumerGroup,
			redis_util.PendingTimeout,
		)

		if err != nil || len(pendingMsgs) == 0 {
			continue
		}

		fmt.Printf("🔄 发现 %d 条待重试消息\n", len(pendingMsgs))

		for _, msg := range pendingMsgs {
			c.processMessage(ctx, msg, "retry-worker")
		}
	}
}

// getGroupMembers 获取群成员
func (c *GroupMsgConsumer) getGroupMembers(ctx context.Context, groupID uint64) ([]*model.GroupMember, error) {
	// 这里需要实现获取群成员的逻辑
	// 暂时返回一个空切片，你需要补充 repository 方法
	return []*model.GroupMember{}, nil
}
