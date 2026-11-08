package messaging

import (
	"context"
	"errors"
	"testing"

	"ginChatProject/internal/platform/rabbitmq"
)

func TestMessageEventProcessorRejectsInvalidEvent(t *testing.T) {
	processor := NewMessageEventProcessor(nil, nil)
	tests := []struct {
		name     string
		envelope rabbitmq.EventEnvelope
	}{
		{
			name: "事件类型不支持",
			envelope: rabbitmq.EventEnvelope{
				EventType: "message.unknown.v1",
			},
		},
		{
			name: "Event ID 非法",
			envelope: rabbitmq.EventEnvelope{
				EventType:   rabbitmq.MessageCreatedRoutingKey,
				EventID:     "invalid",
				AggregateID: "2",
			},
		},
		{
			name: "载荷 JSON 非法",
			envelope: rabbitmq.EventEnvelope{
				EventType:   rabbitmq.MessageCreatedRoutingKey,
				EventID:     "1",
				AggregateID: "2",
				Payload:     []byte(`{"message_id":`),
			},
		},
		{
			name: "聚合 ID 与载荷不一致",
			envelope: rabbitmq.EventEnvelope{
				EventType:   rabbitmq.MessageCreatedRoutingKey,
				EventID:     "1",
				AggregateID: "2",
				Payload: []byte(
					`{"message_id":"3","conversation_id":"4",` +
						`"sender_id":"5","seq":1}`,
				),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := processor.Process(context.Background(), tt.envelope)
			if !errors.Is(err, ErrInvalidEvent) {
				t.Fatalf("Process() 错误 = %v，期望 ErrInvalidEvent", err)
			}
			var permanent interface{ Permanent() bool }
			if !errors.As(err, &permanent) || !permanent.Permanent() {
				t.Fatalf("Process() 错误 = %v，期望永久失败", err)
			}
		})
	}
}
