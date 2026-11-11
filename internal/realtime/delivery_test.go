package realtime

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/PYang176/RelayIM/internal/config"
	"github.com/PYang176/RelayIM/internal/platform/rabbitmq"
)

type stubRouteResolver struct {
	routes map[uint64][]string
	err    error
}

// QueryInstances 查询Instances
func (s stubRouteResolver) QueryInstances(
	context.Context,
	[]uint64,
) (map[uint64][]string, error) {
	return s.routes, s.err
}

type publishedCommand struct {
	exchange   string
	routingKey string
	messageID  string
	body       []byte
}

type stubRawPublisher struct {
	commands []publishedCommand
	err      error
}

// PublishRaw 发布Raw并等待消息系统确认
func (s *stubRawPublisher) PublishRaw(
	_ context.Context,
	exchange string,
	routingKey string,
	messageID string,
	_ string,
	_ time.Time,
	body []byte,
) error {
	s.commands = append(s.commands, publishedCommand{
		exchange:   exchange,
		routingKey: routingKey,
		messageID:  messageID,
		body:       append([]byte(nil), body...),
	})
	return s.err
}

// TestDeliveryRouterMessageIDsUniqueAcrossRouters 覆盖并行消费者共享发布目标时的消息 ID 唯一性
func TestDeliveryRouterMessageIDsUniqueAcrossRouters(t *testing.T) {
	resolver := stubRouteResolver{
		routes: map[uint64][]string{2: {"instance-a"}},
	}
	publisherA := &stubRawPublisher{}
	publisherB := &stubRawPublisher{}
	routerA := NewDeliveryRouter(
		resolver,
		config.RabbitMQConfig{PublishTimeoutSeconds: 1},
		"router-instance",
	)
	routerB := NewDeliveryRouter(
		resolver,
		config.RabbitMQConfig{PublishTimeoutSeconds: 1},
		"router-instance",
	)
	routerA.newPublisher = func() (rawPublisher, error) {
		return publisherA, nil
	}
	routerB.newPublisher = func() (rawPublisher, error) {
		return publisherB, nil
	}
	payload := []byte(`{"action":"message","message_id":"10"}`)
	if err := routerA.Route(context.Background(), []uint64{2}, payload); err != nil {
		t.Fatalf("第一个路由器发布失败: %v", err)
	}
	if err := routerB.Route(context.Background(), []uint64{2}, payload); err != nil {
		t.Fatalf("第二个路由器发布失败: %v", err)
	}
	if len(publisherA.commands) != 1 || len(publisherB.commands) != 1 {
		t.Fatal("每个路由器都应发布一条节点投递命令")
	}
	if publisherA.commands[0].messageID == publisherB.commands[0].messageID {
		t.Fatalf(
			"不同路由器生成了重复消息 ID: %s",
			publisherA.commands[0].messageID,
		)
	}
}

// Close 关闭并释放底层资源
func (s *stubRawPublisher) Close() error {
	return nil
}

// TestDeliveryRouterGroupsUsersByInstance 覆盖实时投递按目标实例聚合用户
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

// TestDeliveryRouterIgnoresUnroutableNode 覆盖节点已退出时实时投递的降级处理
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

// TestDeliveryRouterReturnsResolverError 覆盖实例路由查询失败向调用链传播
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
