package consumer

import (
	"context"
	"encoding/json"
	"fmt"
	"ginChatProject/internal/model"
	"ginChatProject/internal/platform/redisstore"
	"ginChatProject/internal/platform/workerpool"
	"ginChatProject/internal/repository"
	"ginChatProject/internal/ws"
	"github.com/redis/go-redis/v9"
	"time"
)

type GroupMsgConsumer struct {
	msgRepo    *repository.MessageRepository
	workerPool *workerpool.WorkerPool //  加入线程池
	groupRepo  *repository.GroupRepository
}

func NewGroupMsgConsumer(msgRepo *repository.MessageRepository, groupRepo *repository.GroupRepository) *GroupMsgConsumer {
	// 创建线程池：10个worker，队列容量1000
	wp := workerpool.NewWorkerPool(10, 1000)

	return &GroupMsgConsumer{
		msgRepo:    msgRepo,
		workerPool: wp,
		groupRepo:  groupRepo,
	}
}

// Start 启动消费者
func (c *GroupMsgConsumer) Start() {
	ctx := context.Background()

	// 初始化 Redis Stream
	if err := redisstore.InitStream(ctx); err != nil {
		panic(fmt.Sprintf("初始化 Redis Stream 失败: %v", err))
	}

	// 启动线程池
	c.workerPool.Start()

	// 启动多个消费者实例
	for i := 0; i < 3; i++ {
		go c.consume(i)
	}

	// 启动重试机制
	go c.retryFailedMessages()
}

// Stop 停止消费者
func (c *GroupMsgConsumer) Stop() {
	c.workerPool.Stop()
}

// consume 消费消息
func (c *GroupMsgConsumer) consume(workerID int) {
	ctx := context.Background()
	consumerName := fmt.Sprintf("worker-%d", workerID)

	fmt.Printf(" 消费者 %s 启动\n", consumerName)

	for {
		// 读取消息
		messages, err := redisstore.ConsumeMessages(
			ctx,
			redisstore.GroupMsgStream,
			redisstore.GroupMsgConsumerGroup,
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
	// 解析消息
	messageID := msg.Values["message_id"].(string)
	payload := msg.Values["payload"].(string)

	var message model.Message
	if err := json.Unmarshal([]byte(payload), &message); err != nil {
		fmt.Printf("消息解析失败: %v\n", err)
		return
	}

	// 获取群成员
	members, err := c.getGroupMembers(ctx, message.ReceiverID)
	if err != nil {
		fmt.Printf("获取群成员失败: %v\n", err)
		return
	}

	// 为每个群成员创建推送任务
	for _, member := range members {
		if member.UserID == message.SenderID {
			continue
		}

		// 只给在线用户推送
		if !ws.Manager.IsOnline(uint(member.UserID)) {
			continue
		}

		// 创建推送任务并提交给线程池
		task := &workerpool.Task{
			ID: fmt.Sprintf("push_%s_to_%d", messageID, member.UserID),
			Execute: func() error {
				return c.pushToUser(message, member.UserID)
			},
			OnSuccess: func() {
				// 更新消息状态（可选）
				c.msgRepo.UpdateStatus(ctx, messageID, model.MsgDelivered)
			},
			OnError: func(err error) {
				fmt.Printf("推送消息 %s 给用户 %d 失败: %v\n", messageID, member.UserID, err)
			},
		}

		// 提交任务
		if !c.workerPool.Submit(task) {
			fmt.Printf(" 推送任务 %s 被丢弃（队列满）\n", task.ID)
		}
	}

	// 确认消息已处理
	if err := redisstore.AcknowledgeMessage(
		ctx,
		redisstore.GroupMsgStream,
		redisstore.GroupMsgConsumerGroup,
		msg.ID,
	); err != nil {
		fmt.Printf("确认消息失败: %v\n", err)
	}
}

// pushToUser 推送消息给单个用户
func (c *GroupMsgConsumer) pushToUser(message model.Message, userID uint64) error {
	pushMsg := ws.WsMessage{
		Action:     ws.ActionMessage,
		MessageID:  message.MessageID,
		SenderID:   message.SenderID,
		ReceiverID: message.ReceiverID,
		MsgType:    int(message.MsgType),
		Content:    message.Content,
		URL:        message.URL,
	}

	data, err := json.Marshal(pushMsg)
	if err != nil {
		return fmt.Errorf("序列化消息失败: %v", err)
	}

	if !ws.Manager.Send(uint(userID), data) {
		return fmt.Errorf("用户 %d 不在线", userID)
	}

	return nil
}

// retryFailedMessages 重试失败的消息
func (c *GroupMsgConsumer) retryFailedMessages() {
	ctx := context.Background()

	for {
		time.Sleep(30 * time.Second) // 每30秒重试一次

		pendingMsgs, err := redisstore.GetPendingMessages(
			ctx,
			redisstore.GroupMsgStream,
			redisstore.GroupMsgConsumerGroup,
			redisstore.PendingTimeout,
		)

		if err != nil || len(pendingMsgs) == 0 {
			continue
		}

		fmt.Printf(" 发现 %d 条待重试消息\n", len(pendingMsgs))

		for _, msg := range pendingMsgs {
			c.processMessage(ctx, msg, "retry-worker")
		}
	}
}

// getGroupMembers 获取群成员
func (c *GroupMsgConsumer) getGroupMembers(ctx context.Context, groupID uint64) ([]*model.GroupMember, error) {
	return c.groupRepo.GetGroupMembers(ctx, groupID)
}
