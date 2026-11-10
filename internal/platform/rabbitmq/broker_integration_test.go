package rabbitmq

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func TestPublisherIntegration(t *testing.T) {
	rabbitURL := os.Getenv("GINCHAT_TEST_RABBITMQ_URL")
	if rabbitURL == "" {
		t.Skip("未设置 GINCHAT_TEST_RABBITMQ_URL，跳过 RabbitMQ 集成测试")
	}

	publisher, err := NewPublisher(rabbitURL)
	if err != nil {
		t.Fatalf("创建 RabbitMQ 发布器失败: %v", err)
	}
	t.Cleanup(func() {
		if err := publisher.Close(); err != nil {
			t.Errorf("关闭 RabbitMQ 发布器失败: %v", err)
		}
	})

	connection, err := amqp.Dial(rabbitURL)
	if err != nil {
		t.Fatalf("创建测试消费连接失败: %v", err)
	}
	t.Cleanup(func() {
		if err := connection.Close(); err != nil && err != amqp.ErrClosed {
			t.Errorf("关闭测试消费连接失败: %v", err)
		}
	})

	channel, err := connection.Channel()
	if err != nil {
		t.Fatalf("创建测试消费 Channel 失败: %v", err)
	}

	routingKey := fmt.Sprintf("message.test.%d", time.Now().UnixNano())
	queue, err := channel.QueueDeclare(
		"",
		false,
		true,
		true,
		false,
		nil,
	)
	if err != nil {
		t.Fatalf("声明测试队列失败: %v", err)
	}
	if err := channel.QueueBind(queue.Name, routingKey, EventsExchange, false, nil); err != nil {
		t.Fatalf("绑定测试队列失败: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)
	previousPropagator := otel.GetTextMapPropagator()
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() {
		otel.SetTextMapPropagator(previousPropagator)
		_ = provider.Shutdown(context.Background())
	})
	ctx, span := provider.Tracer("rabbitmq-test").Start(ctx, "publish")
	defer span.End()
	deliveries, err := channel.ConsumeWithContext(
		ctx,
		queue.Name,
		"publisher-integration-test",
		false,
		true,
		false,
		false,
		nil,
	)
	if err != nil {
		t.Fatalf("启动测试消费者失败: %v", err)
	}

	eventID := uint64(time.Now().UnixNano())
	envelope := EventEnvelope{
		EventID:     strconv.FormatUint(eventID, 10),
		EventType:   routingKey,
		OccurredAt:  time.Now().UTC(),
		AggregateID: "200",
		Payload:     json.RawMessage(`{"message_id":"300"}`),
	}
	if err := publisher.Publish(ctx, envelope); err != nil {
		t.Fatalf("发布测试事件失败: %v", err)
	}

	select {
	case delivery, ok := <-deliveries:
		if !ok {
			t.Fatal("测试消费者 Channel 提前关闭")
		}
		if delivery.MessageId != envelope.EventID {
			t.Fatalf("消息 ID = %q，期望 %q", delivery.MessageId, envelope.EventID)
		}
		if delivery.DeliveryMode != amqp.Persistent {
			t.Fatalf("DeliveryMode = %d，期望持久化", delivery.DeliveryMode)
		}
		if traceParent, _ := delivery.Headers["traceparent"].(string); traceParent == "" {
			t.Fatal("RabbitMQ 消息缺少 traceparent 头")
		}
		if err := delivery.Ack(false); err != nil {
			t.Fatalf("ACK 测试消息失败: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("等待 RabbitMQ 测试消息超时")
	}
}
