package messaging

import (
	"context"
	"errors"
	"testing"
	"time"

	"ginChatProject/internal/config"
	"ginChatProject/internal/platform/rabbitmq"
	amqp "github.com/rabbitmq/amqp091-go"
)

type stubOutboxStore struct {
	events           []outboxEvent
	claimed          bool
	published        []uint64
	retried          []uint64
	dead             []uint64
	retryAvailable   time.Time
	lastErrorMessage string
}

func (s *stubOutboxStore) claimOutbox(
	context.Context,
	string,
	time.Time,
	time.Duration,
	int,
) ([]outboxEvent, error) {
	s.claimed = true
	return s.events, nil
}

func (s *stubOutboxStore) markOutboxPublished(
	_ context.Context,
	eventID uint64,
	_ string,
	_ time.Time,
) error {
	s.published = append(s.published, eventID)
	return nil
}

func (s *stubOutboxStore) retryOutbox(
	_ context.Context,
	eventID uint64,
	_ string,
	availableAt time.Time,
	lastError string,
) error {
	s.retried = append(s.retried, eventID)
	s.retryAvailable = availableAt
	s.lastErrorMessage = lastError
	return nil
}

func (s *stubOutboxStore) markOutboxDead(
	_ context.Context,
	eventID uint64,
	_ string,
	lastError string,
) error {
	s.dead = append(s.dead, eventID)
	s.lastErrorMessage = lastError
	return nil
}

type stubEventPublisher struct {
	err       error
	envelopes []rabbitmq.EventEnvelope
	closed    chan *amqp.Error
}

func (s *stubEventPublisher) Publish(
	_ context.Context,
	envelope rabbitmq.EventEnvelope,
) error {
	s.envelopes = append(s.envelopes, envelope)
	return s.err
}

func (s *stubEventPublisher) WaitClosed() <-chan *amqp.Error {
	return s.closed
}

func (s *stubEventPublisher) Close() error {
	return nil
}

func TestOutboxWorkerDispatchPublished(t *testing.T) {
	createdAt := time.Date(2026, time.November, 7, 9, 0, 0, 0, time.UTC)
	store := &stubOutboxStore{
		events: []outboxEvent{
			{
				ID:           1,
				EventID:      100,
				AggregateID:  200,
				EventType:    "message.created.v1",
				Payload:      `{"message_id":"300"}`,
				AttemptCount: 1,
				CreatedAt:    createdAt,
			},
		},
	}
	publisher := &stubEventPublisher{}
	worker := &OutboxWorker{
		store:    store,
		config:   config.RabbitMQConfig{BatchSize: 10, PublishTimeoutSeconds: 1},
		workerID: "worker-1",
	}

	processed, err := worker.dispatchBatch(context.Background(), publisher)
	if err != nil {
		t.Fatalf("dispatchBatch() 错误: %v", err)
	}
	if processed != 1 {
		t.Fatalf("处理数量 = %d，期望 1", processed)
	}
	if len(store.published) != 1 || store.published[0] != 1 {
		t.Fatalf("成功标记 = %v，期望事件 1", store.published)
	}
	if len(publisher.envelopes) != 1 {
		t.Fatalf("发布事件数量 = %d，期望 1", len(publisher.envelopes))
	}
	envelope := publisher.envelopes[0]
	if envelope.EventID != "100" ||
		envelope.AggregateID != "200" ||
		envelope.EventType != "message.created.v1" ||
		!envelope.OccurredAt.Equal(createdAt) {
		t.Fatalf("事件信封 = %+v", envelope)
	}
}

func TestOutboxWorkerRetriesFailedPublish(t *testing.T) {
	store := &stubOutboxStore{
		events: []outboxEvent{
			{ID: 1, AttemptCount: 2},
		},
	}
	publisher := &stubEventPublisher{err: errors.New("broker unavailable")}
	worker := &OutboxWorker{
		store: store,
		config: config.RabbitMQConfig{
			BatchSize:             10,
			PublishTimeoutSeconds: 1,
			MaxPublishAttempts:    5,
			RetryBaseSeconds:      1,
		},
		workerID: "worker-1",
	}
	before := time.Now().UTC()

	if _, err := worker.dispatchBatch(context.Background(), publisher); err == nil {
		t.Fatal("发布失败应返回错误以触发连接重建")
	}
	if len(store.retried) != 1 || store.retried[0] != 1 {
		t.Fatalf("重试标记 = %v，期望事件 1", store.retried)
	}
	if len(store.dead) != 0 {
		t.Fatalf("未达到最大次数却进入死信: %v", store.dead)
	}
	if store.retryAvailable.Before(before.Add(2 * time.Second)) {
		t.Fatalf("下次重试时间 = %v，期望至少退避 2 秒", store.retryAvailable)
	}
}

func TestOutboxWorkerMarksDeadAfterMaxAttempts(t *testing.T) {
	store := &stubOutboxStore{
		events: []outboxEvent{
			{ID: 1, AttemptCount: 3},
		},
	}
	publisher := &stubEventPublisher{err: errors.New("broker unavailable")}
	worker := &OutboxWorker{
		store: store,
		config: config.RabbitMQConfig{
			BatchSize:             10,
			PublishTimeoutSeconds: 1,
			MaxPublishAttempts:    3,
		},
		workerID: "worker-1",
	}

	processed, err := worker.dispatchBatch(context.Background(), publisher)
	if err != nil {
		t.Fatalf("达到最大次数后应完成死信标记: %v", err)
	}
	if processed != 1 {
		t.Fatalf("处理数量 = %d，期望 1", processed)
	}
	if len(store.dead) != 1 || store.dead[0] != 1 {
		t.Fatalf("死信标记 = %v，期望事件 1", store.dead)
	}
}

func TestOutboxWorkerRetryDelay(t *testing.T) {
	worker := &OutboxWorker{
		config: config.RabbitMQConfig{RetryBaseSeconds: 2},
	}
	tests := []struct {
		attempt int
		want    time.Duration
	}{
		{attempt: 1, want: 2 * time.Second},
		{attempt: 2, want: 4 * time.Second},
		{attempt: 3, want: 8 * time.Second},
	}
	for _, tt := range tests {
		if got := worker.retryDelay(tt.attempt); got != tt.want {
			t.Fatalf("retryDelay(%d) = %v，期望 %v", tt.attempt, got, tt.want)
		}
	}
}
