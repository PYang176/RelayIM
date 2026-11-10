package realtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"relayim/internal/config"
	"relayim/internal/platform/observability"
	"relayim/internal/platform/rabbitmq"
)

const maxDeliveryPayloadBytes = 1 << 20

// NodeDelivery 是一条发往指定 Go 实例的 WebSocket 投递命令。
type NodeDelivery struct {
	// DeliveryID 是节点投递命令的唯一标识。
	DeliveryID string `json:"delivery_id"`

	// UserIDs 是目标实例上应接收该帧的用户 Snowflake ID 字符串。
	UserIDs []string `json:"user_ids"`

	// Payload 是最终发送给浏览器的 WebSocket JSON 帧。
	Payload json.RawMessage `json:"payload"`

	// OccurredAt 是消息事件首次产生时间。
	OccurredAt time.Time `json:"occurred_at"`
}

// RouteResolver 查询用户设备当前连接的 Go 实例。
type RouteResolver interface {
	QueryInstances(
		ctx context.Context,
		userIDs []uint64,
	) (map[uint64][]string, error)
}

type rawPublisher interface {
	PublishRaw(
		ctx context.Context,
		exchange string,
		routingKey string,
		messageID string,
		messageType string,
		occurredAt time.Time,
		body []byte,
	) error
	Close() error
}

type publisherFactory func() (rawPublisher, error)

// DeliveryRouter 将一次领域事件按目标实例拆分为 RabbitMQ 投递命令。
type DeliveryRouter struct {
	routes       RouteResolver
	config       config.RabbitMQConfig
	instanceID   string
	newPublisher publisherFactory
	sequence     atomic.Uint64

	mu        sync.Mutex
	publisher rawPublisher
}

// NewDeliveryRouter 创建跨实例投递路由器。
func NewDeliveryRouter(
	routes RouteResolver,
	cfg config.RabbitMQConfig,
	instanceID string,
) *DeliveryRouter {
	return &DeliveryRouter{
		routes:     routes,
		config:     cfg,
		instanceID: instanceID,
		newPublisher: func() (rawPublisher, error) {
			return rabbitmq.NewPublisher(cfg.URL)
		},
	}
}

// Route 查询目标实例并分别发布节点投递命令。
func (r *DeliveryRouter) Route(
	ctx context.Context,
	userIDs []uint64,
	payload []byte,
) error {
	ctx, span := observability.Tracer().Start(
		ctx,
		"realtime.route",
		trace.WithSpanKind(trace.SpanKindProducer),
		trace.WithAttributes(
			attribute.Int("messaging.recipient_count", len(userIDs)),
		),
	)
	defer span.End()

	if len(userIDs) == 0 || len(payload) == 0 {
		return nil
	}
	if len(payload) > maxDeliveryPayloadBytes {
		return fmt.Errorf("WebSocket 投递载荷超过 1 MB")
	}

	routes, err := r.routes.QueryInstances(ctx, userIDs)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("查询跨实例实时路由: %w", err)
	}
	if len(routes) == 0 {
		return nil
	}

	groups := make(map[string]map[uint64]struct{})
	for userID, instances := range routes {
		for _, instanceID := range instances {
			if instanceID == "" {
				continue
			}
			if groups[instanceID] == nil {
				groups[instanceID] = make(map[uint64]struct{})
			}
			groups[instanceID][userID] = struct{}{}
		}
	}
	instanceIDs := make([]string, 0, len(groups))
	for instanceID := range groups {
		instanceIDs = append(instanceIDs, instanceID)
	}
	sort.Strings(instanceIDs)
	span.SetAttributes(attribute.Int("messaging.instance_count", len(instanceIDs)))

	r.mu.Lock()
	defer r.mu.Unlock()
	for _, instanceID := range instanceIDs {
		if err := r.publishToInstance(
			ctx,
			instanceID,
			groups[instanceID],
			payload,
		); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return err
		}
	}
	return nil
}

// Close 释放长期使用的跨实例发布连接。
func (r *DeliveryRouter) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.publisher == nil {
		return nil
	}
	err := r.publisher.Close()
	r.publisher = nil
	return err
}

// publishToInstance 发布ToInstance并等待消息系统确认
func (r *DeliveryRouter) publishToInstance(
	ctx context.Context,
	targetInstanceID string,
	userSet map[uint64]struct{},
	payload []byte,
) error {
	if r.publisher == nil {
		publisher, err := r.newPublisher()
		if err != nil {
			return fmt.Errorf("连接 RabbitMQ 实时投递发布端: %w", err)
		}
		r.publisher = publisher
	}

	userIDs := make([]string, 0, len(userSet))
	for userID := range userSet {
		userIDs = append(userIDs, strconv.FormatUint(userID, 10))
	}
	sort.Strings(userIDs)
	now := time.Now().UTC()
	deliveryID := fmt.Sprintf(
		"%s-%d",
		r.instanceID,
		r.sequence.Add(1),
	)
	body, err := json.Marshal(NodeDelivery{
		DeliveryID: deliveryID,
		UserIDs:    userIDs,
		Payload:    append(json.RawMessage(nil), payload...),
		OccurredAt: now,
	})
	if err != nil {
		return fmt.Errorf("序列化节点投递命令: %w", err)
	}

	timeout := time.Duration(r.config.PublishTimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	publishCtx, cancel := context.WithTimeout(ctx, timeout)
	err = r.publisher.PublishRaw(
		publishCtx,
		rabbitmq.DeliveryExchange,
		rabbitmq.DeliveryNodeQueuePrefix+targetInstanceID,
		deliveryID,
		"ws.delivery.v1",
		now,
		body,
	)
	cancel()
	if errors.Is(err, rabbitmq.ErrPublishUnroutable) {
		// 实例已经退出或队列尚未建立。实时帧允许丢失，客户端重连后
		// 使用 after_seq 从 MySQL 补拉。
		return nil
	}
	if err != nil {
		_ = r.publisher.Close()
		r.publisher = nil
		return fmt.Errorf("发布实例实时投递命令: %w", err)
	}
	return nil
}
