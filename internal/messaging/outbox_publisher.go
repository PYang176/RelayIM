package messaging

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/PYang176/RelayIM/internal/config"
	"github.com/PYang176/RelayIM/internal/platform/observability"
	"github.com/PYang176/RelayIM/internal/platform/rabbitmq"
	amqp "github.com/rabbitmq/amqp091-go"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const (
	defaultReconnectDelay = 2 * time.Second
	maxRetryDelay         = time.Hour
)

type outboxStore interface {
	claimOutbox(
		ctx context.Context,
		workerID string,
		now time.Time,
		lease time.Duration,
		limit int,
	) ([]outboxEvent, error)
	markOutboxPublished(
		ctx context.Context,
		eventID uint64,
		workerID string,
		publishedAt time.Time,
	) error
	retryOutbox(
		ctx context.Context,
		eventID uint64,
		workerID string,
		availableAt time.Time,
		lastError string,
	) error
	markOutboxDead(
		ctx context.Context,
		eventID uint64,
		workerID string,
		lastError string,
	) error
}

type eventPublisher interface {
	Publish(ctx context.Context, envelope rabbitmq.EventEnvelope) error
	WaitClosed() <-chan *amqp.Error
	Close() error
}

type eventPublisherFactory func() (eventPublisher, error)

// OutboxWorker 轮询 Outbox，并通过 Publisher Confirm 可靠发布事件。
type OutboxWorker struct {
	store        outboxStore
	config       config.RabbitMQConfig
	workerID     string
	newPublisher eventPublisherFactory
}

// NewOutboxWorker 创建 RabbitMQ Outbox 发布器。
func NewOutboxWorker(
	repository *Repository,
	cfg config.RabbitMQConfig,
	workerID string,
) *OutboxWorker {
	return &OutboxWorker{
		store:    repository,
		config:   cfg,
		workerID: workerID,
		newPublisher: func() (eventPublisher, error) {
			return rabbitmq.NewPublisher(cfg.URL)
		},
	}
}

// Run 持续发布 Outbox，直到 Context 被取消。
func (w *OutboxWorker) Run(ctx context.Context) {
	var publisher eventPublisher
	defer func() {
		if publisher != nil {
			if err := publisher.Close(); err != nil {
				slog.Warn("关闭 RabbitMQ 发布器失败", "error", err)
			}
		}
	}()

	for {
		if ctx.Err() != nil {
			return
		}

		if publisher == nil {
			created, err := w.newPublisher()
			if err != nil {
				slog.Warn("连接 RabbitMQ 发布端失败，将重试", "error", err)
				if !waitForReconnect(ctx, defaultReconnectDelay, nil) {
					return
				}
				continue
			}
			publisher = created
		}

		processed, err := w.dispatchBatch(ctx, publisher)
		if err != nil {
			slog.Error("发布 Outbox 事件失败", "error", err)
			if closeErr := publisher.Close(); closeErr != nil {
				slog.Warn("关闭异常 RabbitMQ 发布器失败", "error", closeErr)
			}
			publisher = nil
			if !waitForReconnect(ctx, defaultReconnectDelay, nil) {
				return
			}
			continue
		}

		if processed == 0 {
			delay := time.Duration(w.config.PollIntervalMilliseconds) * time.Millisecond
			if delay <= 0 {
				delay = 500 * time.Millisecond
			}
			if !waitForReconnect(ctx, delay, publisher) {
				if ctx.Err() != nil {
					return
				}
				slog.Warn("RabbitMQ 发布连接已关闭，准备重建")
				if err := publisher.Close(); err != nil {
					slog.Warn("关闭 RabbitMQ 发布连接失败", "error", err)
				}
				publisher = nil
			}
		}
	}
}

// dispatchBatch 领取一批 Outbox 事件并逐条发布
func (w *OutboxWorker) dispatchBatch(
	ctx context.Context,
	publisher eventPublisher,
) (int, error) {
	now := time.Now().UTC()
	events, err := w.store.claimOutbox(
		ctx,
		w.workerID,
		now,
		time.Duration(w.config.ClaimLeaseSeconds)*time.Second,
		w.config.BatchSize,
	)
	if err != nil {
		return 0, err
	}

	for _, event := range events {
		if err := w.publishEvent(ctx, publisher, event); err != nil {
			return 0, err
		}
	}
	return len(events), nil
}

// publishEvent 发布事件并等待消息系统确认
func (w *OutboxWorker) publishEvent(
	ctx context.Context,
	publisher eventPublisher,
	event outboxEvent,
) error {
	traceParent := ""
	if event.TraceParent != nil {
		traceParent = *event.TraceParent
	}
	eventCtx := observability.ContextWithTraceParent(ctx, traceParent)
	eventCtx, span := observability.Tracer().Start(
		eventCtx,
		"outbox.publish "+event.EventType,
		trace.WithSpanKind(trace.SpanKindProducer),
		trace.WithAttributes(
			attribute.Int64("outbox.id", int64(event.ID)),
			attribute.String("messaging.message.id", strconv.FormatUint(
				event.EventID,
				10,
			)),
			attribute.String("messaging.destination.name", event.EventType),
		),
	)
	defer span.End()

	envelope := rabbitmq.EventEnvelope{
		EventID:     strconv.FormatUint(event.EventID, 10),
		EventType:   event.EventType,
		OccurredAt:  event.CreatedAt,
		AggregateID: strconv.FormatUint(event.AggregateID, 10),
		Payload:     json.RawMessage(event.Payload),
	}

	timeout := time.Duration(w.config.PublishTimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	publishCtx, cancel := context.WithTimeout(eventCtx, timeout)
	publishErr := publisher.Publish(publishCtx, envelope)
	cancel()

	if publishErr == nil {
		if err := w.store.markOutboxPublished(
			ctx,
			event.ID,
			w.workerID,
			time.Now().UTC(),
		); err != nil {
			return err
		}
		return nil
	}
	span.RecordError(publishErr)
	span.SetStatus(codes.Error, publishErr.Error())

	lastError := publishErr.Error()
	maxAttempts := w.config.MaxPublishAttempts
	if maxAttempts <= 0 {
		maxAttempts = 10
	}
	if event.AttemptCount >= maxAttempts {
		if err := w.store.markOutboxDead(
			ctx,
			event.ID,
			w.workerID,
			lastError,
		); err != nil {
			return fmt.Errorf("标记 Outbox 死信: %w", err)
		}
		slog.Error(
			"Outbox 事件进入死信状态",
			"outbox_id", event.ID,
			"error", lastError,
		)
		return nil
	}

	availableAt := time.Now().UTC().Add(w.retryDelay(event.AttemptCount))
	if err := w.store.retryOutbox(
		ctx,
		event.ID,
		w.workerID,
		availableAt,
		lastError,
	); err != nil {
		return fmt.Errorf("安排 Outbox 重试: %w", err)
	}
	return publishErr
}

// retryDelay 计算 Outbox 发布失败的退避延迟
func (w *OutboxWorker) retryDelay(attempt int) time.Duration {
	baseSeconds := w.config.RetryBaseSeconds
	if baseSeconds <= 0 {
		baseSeconds = 2
	}
	delay := time.Duration(baseSeconds) * time.Second
	for current := 1; current < attempt; current++ {
		if delay >= maxRetryDelay/2 {
			return maxRetryDelay
		}
		delay *= 2
	}
	if delay > maxRetryDelay {
		return maxRetryDelay
	}
	return delay
}

// waitForReconnect 等待发布器重连或上下文取消
func waitForReconnect(
	ctx context.Context,
	delay time.Duration,
	publisher eventPublisher,
) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()

	if publisher == nil {
		select {
		case <-ctx.Done():
			return false
		case <-timer.C:
			return true
		}
	}

	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	case <-publisher.WaitClosed():
		return false
	}
}
