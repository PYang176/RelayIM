package messaging

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"ginChatProject/internal/config"
	"ginChatProject/internal/platform/rabbitmq"
	amqp "github.com/rabbitmq/amqp091-go"
)

const maxEventBodyBytes = 1 << 20

type deliveryConsumer interface {
	Consume(
		ctx context.Context,
		queue string,
		consumerTag string,
		handler rabbitmq.DeliveryHandler,
	) error
	Close() error
}

type consumerFactory func() (deliveryConsumer, error)

// MessageCreatedConsumer 负责连接重建、事件解码和幂等业务处理。
type MessageCreatedConsumer struct {
	processor   *MessageCreatedProcessor
	config      config.RabbitMQConfig
	consumerTag string
	newConsumer consumerFactory
}

// NewMessageCreatedConsumer 创建消息创建事件消费者。
func NewMessageCreatedConsumer(
	processor *MessageCreatedProcessor,
	cfg config.RabbitMQConfig,
	consumerTag string,
) *MessageCreatedConsumer {
	return &MessageCreatedConsumer{
		processor:   processor,
		config:      cfg,
		consumerTag: consumerTag,
		newConsumer: func() (deliveryConsumer, error) {
			return rabbitmq.NewConsumer(cfg.URL, cfg.Prefetch)
		},
	}
}

// Run 持续消费消息创建事件，直到 Context 被取消。
func (c *MessageCreatedConsumer) Run(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			return
		}

		consumer, err := c.newConsumer()
		if err != nil {
			log.Printf("连接 RabbitMQ 消费端失败，将重试: %v", err)
			if !waitForReconnect(ctx, defaultReconnectDelay, nil) {
				return
			}
			continue
		}

		err = consumer.Consume(
			ctx,
			rabbitmq.MessageFanoutQueue,
			c.consumerTag,
			c.processDelivery,
		)
		if closeErr := consumer.Close(); closeErr != nil {
			log.Printf("关闭 RabbitMQ 消费连接失败: %v", closeErr)
		}
		if ctx.Err() != nil {
			return
		}
		log.Printf("RabbitMQ Consumer 停止，将重建连接: %v", err)
		if !waitForReconnect(ctx, defaultReconnectDelay, nil) {
			return
		}
	}
}

func (c *MessageCreatedConsumer) processDelivery(
	ctx context.Context,
	delivery amqp.Delivery,
) error {
	if len(delivery.Body) > maxEventBodyBytes {
		return permanentEventError{err: fmt.Errorf(
			"%w: 事件体超过 1 MB",
			ErrInvalidEvent,
		)}
	}

	var envelope rabbitmq.EventEnvelope
	if err := json.Unmarshal(delivery.Body, &envelope); err != nil {
		return permanentEventError{err: fmt.Errorf(
			"%w: 解析事件信封: %v",
			ErrInvalidEvent,
			err,
		)}
	}
	return c.processor.Process(ctx, envelope)
}
