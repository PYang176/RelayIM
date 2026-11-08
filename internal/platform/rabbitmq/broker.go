// Package rabbitmq 封装项目使用的 RabbitMQ 拓扑、可靠发布和消费连接。
package rabbitmq

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	// EventsExchange 接收业务领域事件。
	EventsExchange = "im.events"

	// DeadLetterExchange 接收无法正常处理的事件。
	DeadLetterExchange = "im.dlx"

	// DeliveryExchange 接收跨实例 WebSocket 定向投递命令。
	DeliveryExchange = "im.delivery"

	// DeliveryNodeQueuePrefix 是每个 Go 实例独占的临时投递队列前缀。
	DeliveryNodeQueuePrefix = "im.delivery.node."

	// MessageFanoutQueue 消费消息领域事件并执行后续投递扇出。
	MessageFanoutQueue = "im.message.fanout.v1"

	// MessageDeadLetterQueue 保存最终无法处理的事件。
	MessageDeadLetterQueue = "im.message.dlq.v1"

	// MessageRetry5sQueue 保存第一次失败后延迟 5 秒重试的事件。
	MessageRetry5sQueue = "im.message.retry.5s.v1"

	// MessageRetry30sQueue 保存第二次失败后延迟 30 秒重试的事件。
	MessageRetry30sQueue = "im.message.retry.30s.v1"

	// MessageRetry2mQueue 保存第三次失败后延迟 2 分钟重试的事件。
	MessageRetry2mQueue = "im.message.retry.2m.v1"

	// MessageCreatedRoutingKey 是文本消息创建完成后的领域事件路由键。
	MessageCreatedRoutingKey = "message.created.v1"

	// MessageRecalledRoutingKey 是消息撤回完成后的领域事件路由键。
	MessageRecalledRoutingKey = "message.recalled.v1"

	// MessageDeadLetterRoutingKey 是消息事件进入死信队列时使用的路由键。
	MessageDeadLetterRoutingKey = "im.message.dlq.v1"
)

var (
	// ErrPublishNacked 表示 RabbitMQ 明确拒绝了发布。
	ErrPublishNacked = errors.New("RabbitMQ 拒绝消息发布")

	// ErrPublishUnroutable 表示 mandatory 消息没有匹配到任何队列。
	ErrPublishUnroutable = errors.New("RabbitMQ 消息不可路由")
)

// EventEnvelope 是所有业务事件进入 RabbitMQ 时使用的统一信封。
type EventEnvelope struct {
	// EventID 是事件幂等 Snowflake ID。
	EventID string `json:"event_id"`

	// EventType 是带版本号的事件类型。
	EventType string `json:"event_type"`

	// OccurredAt 是事件首次产生时的 UTC 时间。
	OccurredAt time.Time `json:"occurred_at"`

	// AggregateID 是事件所属聚合 Snowflake ID。
	AggregateID string `json:"aggregate_id"`

	// Payload 是事件最小业务载荷。
	Payload json.RawMessage `json:"payload"`
}

// DeclareTopology 幂等声明当前阶段使用的 Exchange、Queue 和绑定。
func DeclareTopology(channel *amqp.Channel) error {
	if err := channel.ExchangeDeclare(
		EventsExchange,
		amqp.ExchangeTopic,
		true,
		false,
		false,
		false,
		nil,
	); err != nil {
		return fmt.Errorf("声明业务事件 Exchange: %w", err)
	}

	if err := channel.ExchangeDeclare(
		DeadLetterExchange,
		amqp.ExchangeDirect,
		true,
		false,
		false,
		false,
		nil,
	); err != nil {
		return fmt.Errorf("声明死信 Exchange: %w", err)
	}

	if err := channel.ExchangeDeclare(
		DeliveryExchange,
		amqp.ExchangeDirect,
		true,
		false,
		false,
		false,
		nil,
	); err != nil {
		return fmt.Errorf("声明实时投递 Exchange: %w", err)
	}

	if _, err := channel.QueueDeclare(
		MessageFanoutQueue,
		true,
		false,
		false,
		false,
		amqp.Table{
			"x-dead-letter-exchange":    DeadLetterExchange,
			"x-dead-letter-routing-key": MessageDeadLetterRoutingKey,
		},
	); err != nil {
		return fmt.Errorf("声明消息扇出队列: %w", err)
	}

	if _, err := channel.QueueDeclare(
		MessageDeadLetterQueue,
		true,
		false,
		false,
		false,
		nil,
	); err != nil {
		return fmt.Errorf("声明消息死信队列: %w", err)
	}

	retryQueues := []struct {
		name string
		ttl  int32
	}{
		{name: MessageRetry5sQueue, ttl: 5000},
		{name: MessageRetry30sQueue, ttl: 30000},
		{name: MessageRetry2mQueue, ttl: 120000},
	}
	for _, retryQueue := range retryQueues {
		if _, err := channel.QueueDeclare(
			retryQueue.name,
			true,
			false,
			false,
			false,
			amqp.Table{
				"x-message-ttl":             retryQueue.ttl,
				"x-dead-letter-exchange":    EventsExchange,
				"x-dead-letter-routing-key": MessageCreatedRoutingKey,
			},
		); err != nil {
			return fmt.Errorf("声明消息重试队列 %s: %w", retryQueue.name, err)
		}
	}

	if err := channel.QueueBind(
		MessageFanoutQueue,
		MessageCreatedRoutingKey,
		EventsExchange,
		false,
		nil,
	); err != nil {
		return fmt.Errorf("绑定消息创建事件: %w", err)
	}

	if err := channel.QueueBind(
		MessageFanoutQueue,
		MessageRecalledRoutingKey,
		EventsExchange,
		false,
		nil,
	); err != nil {
		return fmt.Errorf("绑定消息撤回事件: %w", err)
	}

	if err := channel.QueueBind(
		MessageDeadLetterQueue,
		MessageDeadLetterRoutingKey,
		DeadLetterExchange,
		false,
		nil,
	); err != nil {
		return fmt.Errorf("绑定消息死信队列: %w", err)
	}
	return nil
}

// Publisher 持有一个只用于发布的连接和 Confirm Channel。
type Publisher struct {
	connection *amqp.Connection
	channel    *amqp.Channel
	returns    <-chan amqp.Return
	closed     <-chan *amqp.Error
}

// NewPublisher 建立发布连接、声明拓扑并开启 Publisher Confirm。
func NewPublisher(url string) (*Publisher, error) {
	connection, err := amqp.DialConfig(url, amqp.Config{
		Heartbeat: 10 * time.Second,
		Locale:    "en_US",
		Dial:      amqp.DefaultDial(5 * time.Second),
	})
	if err != nil {
		return nil, fmt.Errorf("连接 RabbitMQ 发布端: %w", err)
	}

	channel, err := connection.Channel()
	if err != nil {
		_ = connection.Close()
		return nil, fmt.Errorf("创建 RabbitMQ 发布 Channel: %w", err)
	}
	if err := DeclareTopology(channel); err != nil {
		_ = channel.Close()
		_ = connection.Close()
		return nil, err
	}
	if err := channel.Confirm(false); err != nil {
		_ = channel.Close()
		_ = connection.Close()
		return nil, fmt.Errorf("开启 RabbitMQ Publisher Confirm: %w", err)
	}

	return &Publisher{
		connection: connection,
		channel:    channel,
		returns:    channel.NotifyReturn(make(chan amqp.Return, 1)),
		closed:     channel.NotifyClose(make(chan *amqp.Error, 1)),
	}, nil
}

// Publish 发布持久化事件并等待 Broker Confirm。
func (p *Publisher) Publish(
	ctx context.Context,
	envelope EventEnvelope,
) error {
	body, err := json.Marshal(envelope)
	if err != nil {
		return fmt.Errorf("序列化 RabbitMQ 事件: %w", err)
	}
	return p.PublishRaw(
		ctx,
		EventsExchange,
		envelope.EventType,
		envelope.EventID,
		envelope.EventType,
		envelope.OccurredAt,
		body,
	)
}

// PublishRaw 向指定 Exchange 和 Routing Key 发布持久化 JSON 消息。
func (p *Publisher) PublishRaw(
	ctx context.Context,
	exchange string,
	routingKey string,
	messageID string,
	messageType string,
	occurredAt time.Time,
	body []byte,
) error {
	confirmation, err := p.channel.PublishWithDeferredConfirmWithContext(
		ctx,
		exchange,
		routingKey,
		true,
		false,
		amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
			MessageId:    messageID,
			Timestamp:    occurredAt,
			Type:         messageType,
			AppId:        "ginchat",
			Body:         body,
		},
	)
	if err != nil {
		return fmt.Errorf("发布 RabbitMQ 事件: %w", err)
	}
	if confirmation == nil {
		return errors.New("RabbitMQ 发布 Channel 未开启 Confirm")
	}

	acknowledged, err := confirmation.WaitContext(ctx)
	if err != nil {
		return fmt.Errorf("等待 RabbitMQ Publisher Confirm: %w", err)
	}
	if !acknowledged {
		return ErrPublishNacked
	}

	// mandatory 消息不可路由时先收到 basic.return，再收到 Publisher ACK。
	select {
	case returned := <-p.returns:
		return fmt.Errorf(
			"%w: code=%d reply=%s",
			ErrPublishUnroutable,
			returned.ReplyCode,
			returned.ReplyText,
		)
	default:
		return nil
	}
}

// WaitClosed 返回 Channel 关闭通知，供后台发布器重建连接。
func (p *Publisher) WaitClosed() <-chan *amqp.Error {
	return p.closed
}

// Close 释放发布 Channel 和连接。
func (p *Publisher) Close() error {
	var closeErr error
	if p.channel != nil {
		if err := p.channel.Close(); err != nil && !errors.Is(err, amqp.ErrClosed) {
			closeErr = err
		}
	}
	if p.connection != nil {
		if err := p.connection.Close(); err != nil && !errors.Is(err, amqp.ErrClosed) {
			closeErr = errors.Join(closeErr, err)
		}
	}
	return closeErr
}
