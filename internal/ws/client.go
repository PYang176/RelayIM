package ws

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	defaultSendQueueSize = 64
	writeWait            = 5 * time.Second
	pongWait             = 60 * time.Second
	pingPeriod           = 25 * time.Second
	maxIncomingFrameSize = 64 << 10
)

// MessageHandler 处理客户端发来的原始 WebSocket 帧。
type MessageHandler func(ctx context.Context, message []byte) error

// Client 表示一个 WebSocket 连接，所有写操作只能由 writePump 执行。
type Client struct {
	userID    uint64
	deviceID  uint64
	conn      *websocket.Conn
	send      chan []byte
	done      chan struct{}
	closeOnce sync.Once
	onPong    func(context.Context)
}

// NewClient 创建单写协程客户端。
func NewClient(
	userID uint64,
	deviceID uint64,
	conn *websocket.Conn,
	onPong func(context.Context),
) *Client {
	return &Client{
		userID:   userID,
		deviceID: deviceID,
		conn:     conn,
		send:     make(chan []byte, defaultSendQueueSize),
		done:     make(chan struct{}),
		onPong:   onPong,
	}
}

// UserID 返回连接对应的 Snowflake 用户 ID。
func (c *Client) UserID() uint64 {
	return c.userID
}

// DeviceID 返回连接绑定设备 Snowflake ID。
func (c *Client) DeviceID() uint64 {
	return c.deviceID
}

// Enqueue 非阻塞提交消息；队列满时关闭慢连接，避免阻塞其他用户。
func (c *Client) Enqueue(message []byte) bool {
	select {
	case <-c.done:
		return false
	default:
	}

	payload := append([]byte(nil), message...)
	select {
	case c.send <- payload:
		return true
	case <-c.done:
		return false
	default:
		if err := c.Close(); err != nil {
			slog.Warn(
				"关闭慢 WebSocket 连接失败",
				"user_id", c.userID,
				"error", err,
			)
		}
		return false
	}
}

// Run 启动唯一写协程并阻塞读取，直到连接关闭或 Context 取消。
func (c *Client) Run(
	ctx context.Context,
	onMessage MessageHandler,
) {
	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		c.writePump(ctx)
	}()

	c.readPump(ctx, onMessage)
	if err := c.Close(); err != nil {
		slog.Warn(
			"关闭 WebSocket 连接失败",
			"user_id", c.userID,
			"error", err,
		)
	}
	<-writerDone
}

// readPump 持续读取 WebSocket 消息，并在连接结束时退出
func (c *Client) readPump(
	ctx context.Context,
	onMessage MessageHandler,
) {
	c.conn.SetReadLimit(maxIncomingFrameSize)
	if err := c.conn.SetReadDeadline(time.Now().Add(pongWait)); err != nil {
		return
	}
	c.conn.SetPongHandler(func(string) error {
		if err := c.conn.SetReadDeadline(time.Now().Add(pongWait)); err != nil {
			return err
		}
		if c.onPong != nil {
			c.onPong(ctx)
		}
		return nil
	})

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			return
		}
		if onMessage != nil {
			if err := onMessage(ctx, message); err != nil {
				slog.Warn(
					"处理 WebSocket 消息失败",
					"user_id", c.userID,
					"error", err,
				)
			}
		}
	}
}

// writePump 持续把有界队列中的帧写入 WebSocket 连接
func (c *Client) writePump(ctx context.Context) {
	ticker := time.NewTicker(pingPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			_ = c.Close()
			return
		case <-c.done:
			_ = c.conn.WriteControl(
				websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
				time.Now().Add(writeWait),
			)
			return
		case message := <-c.send:
			if err := c.conn.SetWriteDeadline(time.Now().Add(writeWait)); err != nil {
				_ = c.Close()
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				_ = c.Close()
				return
			}
		case <-ticker.C:
			if err := c.conn.WriteControl(
				websocket.PingMessage,
				nil,
				time.Now().Add(writeWait),
			); err != nil {
				_ = c.Close()
				return
			}
		}
	}
}

// Close 幂等关闭连接；发送队列不关闭，避免多个生产者关闭同一 Channel。
func (c *Client) Close() error {
	var closeErr error
	c.closeOnce.Do(func() {
		close(c.done)
		closeErr = c.conn.Close()
	})
	if closeErr != nil && !errors.Is(closeErr, websocket.ErrCloseSent) {
		return closeErr
	}
	return nil
}
