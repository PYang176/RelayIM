package realtime

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/PYang176/RelayIM/internal/config"
	amqp "github.com/rabbitmq/amqp091-go"
)

type stubLocalSender struct {
	userIDs  []uint64
	messages [][]byte
}

// SendToUsers 执行To用户流程并完成持久化准备
func (s *stubLocalSender) SendToUsers(userIDs []uint64, message []byte) int {
	s.userIDs = append([]uint64(nil), userIDs...)
	s.messages = append(s.messages, append([]byte(nil), message...))
	return len(userIDs)
}

// TestNodeConsumerDispatchesLocalUsers 覆盖节点投递命令向本实例用户分发
func TestNodeConsumerDispatchesLocalUsers(t *testing.T) {
	local := &stubLocalSender{}
	consumer := NewNodeConsumer(
		config.RabbitMQConfig{},
		"instance-a",
		local,
	)
	body, err := json.Marshal(NodeDelivery{
		DeliveryID: "delivery-1",
		UserIDs:    []string{"2", "3"},
		Payload:    []byte(`{"action":"message","message_id":"10"}`),
	})
	if err != nil {
		t.Fatalf("序列化节点投递命令失败: %v", err)
	}
	if err := consumer.processDelivery(
		context.Background(),
		amqp.Delivery{Body: body},
	); err != nil {
		t.Fatalf("处理节点投递命令失败: %v", err)
	}
	if len(local.userIDs) != 2 || local.userIDs[0] != 2 || local.userIDs[1] != 3 {
		t.Fatalf("本地接收者 = %v", local.userIDs)
	}
	if len(local.messages) != 1 ||
		string(local.messages[0]) != `{"action":"message","message_id":"10"}` {
		t.Fatalf("本地投递载荷 = %q", local.messages)
	}
}

// TestNodeConsumerDropsMalformedCommand 覆盖非法节点投递命令的丢弃策略
func TestNodeConsumerDropsMalformedCommand(t *testing.T) {
	local := &stubLocalSender{}
	consumer := NewNodeConsumer(
		config.RabbitMQConfig{},
		"instance-a",
		local,
	)
	if err := consumer.processDelivery(
		context.Background(),
		amqp.Delivery{Body: []byte(`{"user_ids":["bad"],"payload":{}}`)},
	); err != nil {
		t.Fatalf("非法节点命令应被确认丢弃: %v", err)
	}
	if len(local.messages) != 0 {
		t.Fatal("非法节点命令不应触发本地投递")
	}
}
