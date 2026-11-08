package realtime

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"ginChatProject/internal/config"
	"ginChatProject/internal/platform/rabbitmq"
)

type stubRouteResolver struct {
	routes map[uint64][]string
	err    error
}

func (s stubRouteResolver) QueryInstances(
	context.Context,
	[]uint64,
) (map[uint64][]string, error) {
	return s.routes, s.err
}

type publishedCommand struct {
	exchange   string
	routingKey string
	body       []byte
}

type stubRawPublisher struct {
	commands []publishedCommand
	err      error
}

func (s *stubRawPublisher) PublishRaw(
	_ context.Context,
	exchange string,
	routingKey string,
	_ string,
	_ string,
	_ time.Time,
	body []byte,
) error {
	s.commands = append(s.commands, publishedCommand{
		exchange:   exchange,
		routingKey: routingKey,
		body:       append([]byte(nil), body...),
	})
	return s.err
}

func (s *stubRawPublisher) Close() error {
	return nil
}

func TestDeliveryRouterGroupsUsersByInstance(t *testing.T) {
	resolver := stubRouteResolver{
		routes: map[uint64][]string{
			2: {"instance-a"},
			3: {"instance-a", "instance-b"},
		},
	}
	publisher := &stubRawPublisher{}
	router := NewDeliveryRouter(
		resolver,
		config.RabbitMQConfig{PublishTimeoutSeconds: 1},
		"router-instance",
	)
	router.newPublisher = func() (rawPublisher, error) {
		return publisher, nil
	}
	payload := []byte(`{"action":"message","message_id":"10"}`)
	if err := router.Route(
		context.Background(),
		[]uint64{2, 3, 4},
		payload,
	); err != nil {
		t.Fatalf("路由消息失败: %v", err)
	}
	if len(publisher.commands) != 2 {
		t.Fatalf("节点投递数量 = %d，期望 2", len(publisher.commands))
	}
	if publisher.commands[0].exchange != rabbitmq.DeliveryExchange ||
		publisher.commands[0].routingKey !=
			rabbitmq.DeliveryNodeQueuePrefix+"instance-a" {
		t.Fatalf("实例 A 投递 = %+v", publisher.commands[0])
	}
	if publisher.commands[1].routingKey !=
		rabbitmq.DeliveryNodeQueuePrefix+"instance-b" {
		t.Fatalf("实例 B 投递 = %+v", publisher.commands[1])
	}

	var commandA NodeDelivery
	if err := json.Unmarshal(publisher.commands[0].body, &commandA); err != nil {
		t.Fatalf("解析实例 A 投递命令失败: %v", err)
	}
	if len(commandA.UserIDs) != 2 ||
		commandA.UserIDs[0] != "2" ||
		commandA.UserIDs[1] != "3" ||
		string(commandA.Payload) != string(payload) {
		t.Fatalf("实例 A 投递命令 = %+v", commandA)
	}
}

func TestDeliveryRouterIgnoresUnroutableNode(t *testing.T) {
	publisher := &stubRawPublisher{err: rabbitmq.ErrPublishUnroutable}
	router := NewDeliveryRouter(
		stubRouteResolver{
			routes: map[uint64][]string{2: {"dead-instance"}},
		},
		config.RabbitMQConfig{PublishTimeoutSeconds: 1},
		"router-instance",
	)
	router.newPublisher = func() (rawPublisher, error) {
		return publisher, nil
	}
	if err := router.Route(
		context.Background(),
		[]uint64{2},
		[]byte(`{"action":"message"}`),
	); err != nil {
		t.Fatalf("不可路由节点不应阻塞实时投递: %v", err)
	}
}

func TestDeliveryRouterReturnsResolverError(t *testing.T) {
	router := NewDeliveryRouter(
		stubRouteResolver{err: errors.New("redis unavailable")},
		config.RabbitMQConfig{},
		"router-instance",
	)
	if err := router.Route(
		context.Background(),
		[]uint64{2},
		[]byte(`{}`),
	); err == nil {
		t.Fatal("路由查询失败应返回错误")
	}
}
