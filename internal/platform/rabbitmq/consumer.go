package rabbitmq

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	// MaxDeliveryRetries 是消费失败后进入 TTL 重试队列的最大次数。
	MaxDeliveryRetries    = 3
	deliveryHandleTimeout = 10 * time.Second
)

// DeliveryHandler 处理一条已解码 RabbitMQ 投递，返回值决定 ACK 或重试。
type DeliveryHandler func(ctx context.Context, delivery amqp.Delivery) error

// PermanentError 标记重试也无法修复的业务错误。
type PermanentError interface {
	error
	Permanent() bool
}

// Consumer 持有独立的消费连接、Channel 和 Publisher Confirm。
type Consumer struct {
	connection *amqp.Connection
	channel    *amqp.Channel
}

// NewConsumer 建立消费连接并声明项目拓扑。
func NewConsumer(url string, prefetch int) (*Consumer, error) {
	connection, err := amqp.DialConfig(url, amqp.Config{
		Heartbeat: 10 * time.Second,
		Locale:    "en_US",
		Dial:      amqp.DefaultDial(5 * time.Second),
	})
	if err != nil {
		return nil, fmt.Errorf("连接 RabbitMQ 消费端: %w", err)
	}

	channel, err := connection.Channel()
	if err != nil {
		_ = connection.Close()
		return nil, fmt.Errorf("创建 RabbitMQ 消费 Channel: %w", err)
	}
	if err := DeclareTopology(channel); err != nil {
		_ = connection.Close()
		return nil, err
	}
	if prefetch <= 0 {
		prefetch = 20
	}
	if err := channel.Qos(prefetch, 0, false); err != nil {
		_ = connection.Close()
		return nil, fmt.Errorf("设置 RabbitMQ Consumer Prefetch: %w", err)
	}
	if err := channel.Confirm(false); err != nil {
		_ = connection.Close()
		return nil, fmt.Errorf("开启消费端 Publisher Confirm: %w", err)
	}

	return &Consumer{
		connection: connection,
		channel:    channel,
	}, nil
}

// DeclareNodeQueue 声明当前应用实例独占的临时 WebSocket 投递队列。
func (c *Consumer) DeclareNodeQueue(instanceID string) error {
	instanceID = strings.TrimSpace(instanceID)
	if instanceID == "" {
		return errors.New("实时投递实例 ID 不能为空")
	}
	queue := DeliveryNodeQueuePrefix + instanceID
	if _, err := c.channel.QueueDeclare(
		queue,
		false,
		true,
		true,
		false,
		nil,
	); err != nil {
		return fmt.Errorf("声明实例实时投递队列: %w", err)
	}
	if err := c.channel.QueueBind(
		queue,
		queue,
		DeliveryExchange,
		false,
		nil,
	); err != nil {
		return fmt.Errorf("绑定实例实时投递队列: %w", err)
	}
	return nil
}

// Consume 手动 ACK 处理队列消息，失败消息按重试队列或 DLQ 处理。
func (c *Consumer) Consume(
	ctx context.Context,
	queue string,
	consumerTag string,
	handler DeliveryHandler,
) error {
	deliveries, err := c.channel.ConsumeWithContext(
		ctx,
		queue,
		consumerTag,
		false,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		return fmt.Errorf("启动 RabbitMQ Consumer: %w", err)
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case delivery, ok := <-deliveries:
			if !ok {
				return errors.New("RabbitMQ Consumer Channel 已关闭")
			}
			if err := c.handleDelivery(ctx, delivery, handler); err != nil {
				return err
			}
		}
	}
}

func (c *Consumer) handleDelivery(
	ctx context.Context,
	delivery amqp.Delivery,
	handler DeliveryHandler,
) error {
	handleCtx, cancel := context.WithTimeout(ctx, deliveryHandleTimeout)
	handleErr := handler(handleCtx, delivery)
	cancel()
	if handleErr == nil {
		if err := delivery.Ack(false); err != nil {
			return fmt.Errorf("ACK RabbitMQ 消息失败: %w", err)
		}
		return nil
	}

	var permanent PermanentError
	if errors.As(handleErr, &permanent) && permanent.Permanent() {
		if err := delivery.Nack(false, false); err != nil {
			return fmt.Errorf("NACK 不可重试消息失败: %w", err)
		}
		return nil
	}

	if DeliveryRetryCount(delivery.Headers) >= MaxDeliveryRetries {
		if err := delivery.Nack(false, false); err != nil {
			return fmt.Errorf("NACK 达到重试上限的消息失败: %w", err)
		}
		return nil
	}

	if err := c.retryDelivery(ctx, delivery); err != nil {
		// 重试发布本身失败时重新入主队列；消费端幂等保证重复处理安全。
		if nackErr := delivery.Nack(false, true); nackErr != nil {
			return errors.Join(err, nackErr)
		}
		return nil
	}
	if err := delivery.Ack(false); err != nil {
		return fmt.Errorf("ACK 已转投重试队列的消息失败: %w", err)
	}
	return nil
}

func (c *Consumer) retryDelivery(
	ctx context.Context,
	delivery amqp.Delivery,
) error {
	retryCount := DeliveryRetryCount(delivery.Headers)
	retryQueue := retryQueueForCount(retryCount)
	headers := amqp.Table{}
	for key, value := range delivery.Headers {
		headers[key] = value
	}
	headers["x-retry-count"] = int32(retryCount + 1)

	confirmation, err := c.channel.PublishWithDeferredConfirmWithContext(
		ctx,
		"",
		retryQueue,
		true,
		false,
		amqp.Publishing{
			Headers:       headers,
			ContentType:   delivery.ContentType,
			DeliveryMode:  amqp.Persistent,
			MessageId:     delivery.MessageId,
			CorrelationId: delivery.CorrelationId,
			Timestamp:     delivery.Timestamp,
			Type:          delivery.Type,
			AppId:         delivery.AppId,
			UserId:        delivery.UserId,
			Body:          delivery.Body,
		},
	)
	if err != nil {
		return fmt.Errorf("发布消息到重试队列: %w", err)
	}
	if confirmation == nil {
		return errors.New("RabbitMQ 消费 Channel 未开启 Confirm")
	}
	acknowledged, err := confirmation.WaitContext(ctx)
	if err != nil {
		return fmt.Errorf("等待重试消息 Publisher Confirm: %w", err)
	}
	if !acknowledged {
		return ErrPublishNacked
	}
	return nil
}

// DeliveryRetryCount 读取当前投递已经经过的 TTL 重试次数。
func DeliveryRetryCount(headers amqp.Table) int {
	value, exists := headers["x-retry-count"]
	if !exists {
		return 0
	}
	switch count := value.(type) {
	case int:
		return count
	case int32:
		return int(count)
	case int64:
		return int(count)
	default:
		return 0
	}
}

func retryQueueForCount(retryCount int) string {
	switch retryCount {
	case 0:
		return EventsRetry5sQueue
	case 1:
		return EventsRetry30sQueue
	default:
		return EventsRetry2mQueue
	}
}

// Close 关闭消费连接，Channel 会随连接一并释放。
func (c *Consumer) Close() error {
	if c.connection == nil {
		return nil
	}
	if err := c.connection.Close(); err != nil && !errors.Is(err, amqp.ErrClosed) {
		return err
	}
	return nil
}
