package ws

import (
	"context"
	"encoding/json"
	"errors"
	"ginChatProject/internal/platform/authn"
	"ginChatProject/internal/platform/redisstore"
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

const heartbeatTTL = 70 * time.Second

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     checkOrigin,
}

// WebsocketHandler 处理 WebSocket 鉴权、连接交付和客户端回执。
type WebsocketHandler struct{}

// NewWebsocketHandler 创建 WebSocket Handler。
func NewWebsocketHandler() *WebsocketHandler {
	return &WebsocketHandler{}
}

// Connect 完成鉴权后把连接交给单写协程客户端。
func (h *WebsocketHandler) Connect(c *gin.Context) {
	ticket := c.Query("ticket")
	if ticket == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未提供 WebSocket Ticket"})
		return
	}

	userID, err := authn.DefaultManager.ConsumeWSTicket(
		c.Request.Context(),
		ticket,
	)
	if err != nil {
		if errors.Is(err, authn.ErrAuthUnavailable) {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "认证服务暂不可用"})
			return
		}
		c.JSON(http.StatusForbidden, gin.H{"error": "WebSocket Ticket 无效或已使用"})
		return
	}

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("升级 WebSocket 连接失败: %v", err)
		return
	}

	client := NewClient(
		userID,
		conn,
		func(heartbeatCtx context.Context) {
			h.updateHeartbeat(heartbeatCtx, userID)
		},
	)
	if !Manager.Add(client) {
		_ = conn.WriteControl(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(
				websocket.CloseTryAgainLater,
				"连接数超过限制",
			),
			time.Now().Add(writeWait),
		)
		_ = conn.Close()
		return
	}
	defer Manager.Remove(client)

	ctx, cancel := context.WithCancel(c.Request.Context())
	defer cancel()

	client.Run(ctx, h.handleClientMessage)
}

func (h *WebsocketHandler) handleClientMessage(
	_ context.Context,
	raw []byte,
) error {
	var message WsMessage
	if err := json.Unmarshal(raw, &message); err != nil {
		// 非法帧直接忽略；连接仍由读写超时和 Pong 机制保持或关闭。
		return nil
	}
	// 新版送达和已读尚未迁移到 message_deliveries/last_read_seq，
	// 暂不在 WebSocket Handler 中调用旧 message 表。
	return nil
}

func (h *WebsocketHandler) updateHeartbeat(
	ctx context.Context,
	userID uint64,
) {
	heartbeatCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	key := heartbeatKey(userID)
	if err := redisstore.RDB.Set(
		heartbeatCtx,
		key,
		time.Now().Unix(),
		heartbeatTTL,
	).Err(); err != nil {
		log.Printf("更新 WebSocket 心跳失败: error=%v", err)
	}
}

func heartbeatKey(userID uint64) string {
	return "heartbeat:" + strconv.FormatUint(userID, 10)
}

func checkOrigin(request *http.Request) bool {
	origin := request.Header.Get("Origin")
	if origin == "" {
		return true
	}
	parsed, err := url.Parse(origin)
	if err != nil {
		return false
	}
	if parsed.Host == request.Host {
		return true
	}
	host := parsed.Hostname()
	ip := net.ParseIP(host)
	return host == "localhost" || (ip != nil && ip.IsLoopback())
}
