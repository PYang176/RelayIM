package realtime

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"ginChatProject/internal/config"
	"ginChatProject/internal/platform/rabbitmq"
	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	nodeReconnectDelay = 2 * time.Second
)

// LocalSender 把消息非阻塞提交给本实例 WebSocket 连接。
type LocalSender interface {
	SendToUsers(userIDs []uint64, message []byte) int
}

type nodeDeliveryConsumer interface {
	DeclareNodeQueue(instanceID string) error
	Consume(
		ctx context.Context,
		queue string,
		consumerTag string,
		handler rabbitmq.DeliveryHandler,
	) error
	Close() error
}

type nodeConsumerFactory func() (nodeDeliveryConsumer, error)

// NodeConsumer 消费定向到当前 Go 实例的 WebSocket 投递命令。
type NodeConsumer struct {
	config      config.RabbitMQConfig
	instanceID  string
	consumerTag string
	local       LocalSender
	newConsumer nodeConsumerFactory
}

// NewNodeConsumer 创建当前实例的实时投递消费者。
func NewNodeConsumer(
	cfg config.RabbitMQConfig,
	instanceID string,
	local LocalSender,
) *NodeConsumer {
	return &NodeConsumer{
		config:      cfg,
		instanceID:  instanceID,
		consumerTag: instanceID + "-node-delivery",
		local:       local,
		newConsumer: func() (nodeDeliveryConsumer, error) {
			return rabbitmq.NewConsumer(cfg.URL, cfg.Prefetch)
		},
	}
}

// Run 持续消费节点投递命令，直到 Context 被取消。
func (c *NodeConsumer) Run(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			return
		}
		consumer, err := c.newConsumer()
		if err != nil {
			slog.Warn("连接 RabbitMQ 节点投递消费端失败，将重试", "error", err)
			if !waitNodeReconnect(ctx) {
				return
			}
			continue
		}
		if err := consumer.DeclareNodeQueue(c.instanceID); err != nil {
			_ = consumer.Close()
			slog.Warn("声明 RabbitMQ 节点投递队列失败，将重试", "error", err)
			if !waitNodeReconnect(ctx) {
				return
			}
			continue
		}

		err = consumer.Consume(
			ctx,
			nodeQueueName(c.instanceID),
			c.consumerTag,
			c.processDelivery,
		)
		if closeErr := consumer.Close(); closeErr != nil {
			slog.Warn("关闭 RabbitMQ 节点投递连接失败", "error", closeErr)
		}
		if ctx.Err() != nil {
			return
		}
		slog.Warn("RabbitMQ 节点投递 Consumer 停止，将重建连接", "error", err)
		if !waitNodeReconnect(ctx) {
			return
		}
	}
}

func (c *NodeConsumer) processDelivery(
	ctx context.Context,
	delivery amqp.Delivery,
) error {
	if len(delivery.Body) > maxDeliveryPayloadBytes {
		slog.Warn("丢弃超大节点投递命令", "size", len(delivery.Body))
		return nil
	}
	var command NodeDelivery
	if err := json.Unmarshal(delivery.Body, &command); err != nil {
		slog.Warn("丢弃无法解析的节点投递命令", "error", err)
		return nil
	}
	if command.DeliveryID == "" || len(command.UserIDs) == 0 ||
		len(command.Payload) == 0 {
		slog.Warn("丢弃字段不完整的节点投递命令")
		return nil
	}
	userIDs := make([]uint64, 0, len(command.UserIDs))
	for _, rawUserID := range command.UserIDs {
		userID, err := strconv.ParseUint(rawUserID, 10, 64)
		if err != nil || userID == 0 {
			slog.Warn("丢弃包含非法用户 ID 的节点投递命令")
			return nil
		}
		userIDs = append(userIDs, userID)
	}
	if c.local == nil {
		return nil
	}
	c.local.SendToUsers(userIDs, command.Payload)
	return nil
}

func waitNodeReconnect(ctx context.Context) bool {
	timer := time.NewTimer(nodeReconnectDelay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func nodeQueueName(instanceID string) string {
	return fmt.Sprintf(
		"%s%s",
		rabbitmq.DeliveryNodeQueuePrefix,
		instanceID,
	)
}
