package messaging

import (
	"errors"
	"testing"

	"ginChatProject/internal/platform/rabbitmq"
	amqp "github.com/rabbitmq/amqp091-go"
)

// TestMessageEventResult 覆盖消费者成功、重试和死信结果分类
func TestMessageEventResult(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		headers amqp.Table
		want    string
	}{
		{
			name: "处理成功",
			want: "success",
		},
		{
			name: "永久错误进入死信",
			err: permanentEventError{
				err: errors.New("invalid event"),
			},
			want: "dead",
		},
		{
			name: "普通错误等待重试",
			err:  errors.New("temporary failure"),
			want: "retry",
		},
		{
			name: "达到重试上限进入死信",
			err:  errors.New("temporary failure"),
			headers: amqp.Table{
				"x-retry-count": int32(rabbitmq.MaxDeliveryRetries),
			},
			want: "dead",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := messageEventResult(tt.err, tt.headers); got != tt.want {
				t.Fatalf("messageEventResult() = %q，期望 %q", got, tt.want)
			}
		})
	}
}
