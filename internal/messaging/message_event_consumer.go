package messaging

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/PYang176/RelayIM/internal/config"
	"github.com/PYang176/RelayIM/internal/platform/observability"
	"github.com/PYang176/RelayIM/internal/platform/rabbitmq"
	amqp "github.com/rabbitmq/amqp091-go"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
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

// MessageEventMetrics 接收消费者结果、处理耗时和事件排队延迟。
type MessageEventMetrics interface {
	RecordMessageEvent(
		eventType string,
		result string,
		duration time.Duration,
		lag time.Duration,
	)
}

// EventConsumer 负责连接重建、事件解码和幂等业务处理。
type EventConsumer struct {
	processor   *EventProcessor
	config      config.RabbitMQConfig
	consumerTag string
	newConsumer consumerFactory
	metrics     MessageEventMetrics
}

// NewEventConsumer 创建业务领域事件消费者。
func NewEventConsumer(
	processor *EventProcessor,
	cfg config.RabbitMQConfig,
	consumerTag string,
	metrics MessageEventMetrics,
) *EventConsumer {
	return &EventConsumer{
		processor:   processor,
		config:      cfg,
		consumerTag: consumerTag,
		metrics:     metrics,
		newConsumer: func() (deliveryConsumer, error) {
			return rabbitmq.NewConsumer(cfg.URL, cfg.Prefetch)
		},
	}
}

// Run 持续消费消息领域事件，直到 Context 被取消。
func (c *EventConsumer) Run(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			return
		}

		consumer, err := c.newConsumer()
		if err != nil {
			slog.Warn("连接 RabbitMQ 消费端失败，将重试", "error", err)
			if !waitForReconnect(ctx, defaultReconnectDelay, nil) {
				return
			}
			continue
		}

		err = consumer.Consume(
			ctx,
			rabbitmq.EventsFanoutQueue,
			c.consumerTag,
			c.processDelivery,
		)
		if closeErr := consumer.Close(); closeErr != nil {
			slog.Warn("关闭 RabbitMQ 消费连接失败", "error", closeErr)
		}
		if ctx.Err() != nil {
			return
		}
		slog.Warn("RabbitMQ Consumer 停止，将重建连接", "error", err)
		if !waitForReconnect(ctx, defaultReconnectDelay, nil) {
			return
		}
	}
}

// processDelivery 解码领域事件并完成消费者幂等处理
func (c *EventConsumer) processDelivery(
	ctx context.Context,
	delivery amqp.Delivery,
) error {
	traceParent, _ := delivery.Headers["traceparent"].(string)
	consumerCtx := observability.ContextWithTraceParent(ctx, traceParent)
	consumerCtx, span := observability.Tracer().Start(
		consumerCtx,
		"rabbitmq.consume",
		trace.WithSpanKind(trace.SpanKindConsumer),
		trace.WithAttributes(
			attribute.String("messaging.system", "rabbitmq"),
			attribute.String("messaging.destination.name", delivery.RoutingKey),
		),
	)
	defer span.End()

	startedAt := time.Now()
	if len(delivery.Body) > maxEventBodyBytes {
		err := permanentEventError{err: fmt.Errorf(
			"%w: 事件体超过 1 MB",
			ErrInvalidEvent,
		)}
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		c.recordMetrics("invalid", "dead", startedAt, time.Time{})
		return err
	}

	var envelope rabbitmq.EventEnvelope
	if err := json.Unmarshal(delivery.Body, &envelope); err != nil {
		eventErr := permanentEventError{err: fmt.Errorf(
			"%w: 解析事件信封: %v",
			ErrInvalidEvent,
			err,
		)}
		span.RecordError(eventErr)
		span.SetStatus(codes.Error, eventErr.Error())
		c.recordMetrics("invalid", "dead", startedAt, time.Time{})
		return eventErr
	}
	span.SetName("rabbitmq.consume " + envelope.EventType)
	span.SetAttributes(
		attribute.String("messaging.message.id", envelope.EventID),
		attribute.String("messaging.operation.name", "process"),
	)
	err := c.processor.Process(consumerCtx, envelope)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	c.recordMetrics(
		envelope.EventType,
		messageEventResult(err, delivery.Headers),
		startedAt,
		envelope.OccurredAt,
	)
	return err
}

// recordMetrics 记录指标
func (c *EventConsumer) recordMetrics(
	eventType string,
	result string,
	startedAt time.Time,
	occurredAt time.Time,
) {
	if c.metrics == nil {
		return
	}
	lag := time.Duration(0)
	if !occurredAt.IsZero() {
		lag = startedAt.Sub(occurredAt)
		if lag < 0 {
			lag = 0
		}
	}
	c.metrics.RecordMessageEvent(
		eventType,
		result,
		time.Since(startedAt),
		lag,
	)
}

// messageEventResult 根据错误类型和重试次数归类消费者处理结果
func messageEventResult(err error, headers amqp.Table) string {
	if err == nil {
		return "success"
	}
	var permanent rabbitmq.PermanentError
	if errors.As(err, &permanent) && permanent.Permanent() {
		return "dead"
	}
	if rabbitmq.DeliveryRetryCount(headers) >= rabbitmq.MaxDeliveryRetries {
		return "dead"
	}
	return "retry"
}
