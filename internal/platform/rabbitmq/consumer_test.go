package rabbitmq

import (
	"testing"

	amqp "github.com/rabbitmq/amqp091-go"
)

func TestRetryQueueForCount(t *testing.T) {
	tests := []struct {
		count int
		want  string
	}{
		{count: 0, want: MessageRetry5sQueue},
		{count: 1, want: MessageRetry30sQueue},
		{count: 2, want: MessageRetry2mQueue},
	}
	for _, tt := range tests {
		if got := retryQueueForCount(tt.count); got != tt.want {
			t.Fatalf("retryQueueForCount(%d) = %q，期望 %q", tt.count, got, tt.want)
		}
	}
}

func TestDeliveryRetryCount(t *testing.T) {
	tests := []struct {
		name    string
		headers amqp.Table
		want    int
	}{
		{name: "无头信息", headers: nil, want: 0},
		{name: "int32", headers: amqp.Table{"x-retry-count": int32(2)}, want: 2},
		{name: "int64", headers: amqp.Table{"x-retry-count": int64(1)}, want: 1},
		{name: "类型错误", headers: amqp.Table{"x-retry-count": "2"}, want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DeliveryRetryCount(tt.headers); got != tt.want {
				t.Fatalf("DeliveryRetryCount() = %d，期望 %d", got, tt.want)
			}
		})
	}
}
