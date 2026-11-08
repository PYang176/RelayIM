package ws

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"ginChatProject/internal/platform/authn"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     checkOrigin,
}

// WebsocketHandler 处理 WebSocket 鉴权、连接交付和客户端回执。
type WebsocketHandler struct {
	receipts ReceiptService
	presence PresenceTracker
}

// PresenceTracker 维护用户设备在线状态。
type PresenceTracker interface {
	Touch(ctx context.Context, userID uint64, deviceID uint64) error
	Remove(ctx context.Context, userID uint64, deviceID uint64) error
}

// ReceiptService 处理新版送达和已读回执。
type ReceiptService interface {
	AcknowledgeMessage(
		ctx context.Context,
		userID uint64,
		deviceID uint64,
		messageID uint64,
	) error
	MarkConversationRead(
		ctx context.Context,
		userID uint64,
		conversationID uint64,
		seq uint64,
	) error
}

// NewWebsocketHandler 创建 WebSocket Handler。
func NewWebsocketHandler(
	receipts ReceiptService,
	presence PresenceTracker,
) *WebsocketHandler {
	return &WebsocketHandler{
		receipts: receipts,
		presence: presence,
	}
}

// Connect 完成鉴权后把连接交给单写协程客户端。
func (h *WebsocketHandler) Connect(c *gin.Context) {
	ticket := c.Query("ticket")
	if ticket == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未提供 WebSocket Ticket"})
		return
	}

	ticketSession, err := authn.DefaultManager.ConsumeWSTicket(
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
		slog.Warn("升级 WebSocket 连接失败", "error", err)
		return
	}

	client := NewClient(
		ticketSession.UserID,
		ticketSession.DeviceID,
		conn,
		func(heartbeatCtx context.Context) {
			h.updatePresence(
				heartbeatCtx,
				ticketSession.UserID,
				ticketSession.DeviceID,
			)
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
	h.updatePresence(
		c.Request.Context(),
		ticketSession.UserID,
		ticketSession.DeviceID,
	)
	defer h.removePresence(ticketSession.UserID, ticketSession.DeviceID)

	ctx, cancel := context.WithCancel(c.Request.Context())
	defer cancel()

	client.Run(ctx, func(messageCtx context.Context, raw []byte) error {
		return h.handleClientMessage(
			messageCtx,
			ticketSession.UserID,
			ticketSession.DeviceID,
			raw,
		)
	})
}

func (h *WebsocketHandler) handleClientMessage(
	ctx context.Context,
	userID uint64,
	deviceID uint64,
	raw []byte,
) error {
	var message WsMessage
	if err := json.Unmarshal(raw, &message); err != nil {
		// 非法帧直接忽略；连接仍由读写超时和 Pong 机制保持或关闭。
		return nil
	}
	operationCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	switch message.Action {
	case ActionAck:
		messageID, err := strconv.ParseUint(message.MessageID, 10, 64)
		if err != nil || messageID == 0 {
			return fmt.Errorf("ACK message_id 格式错误")
		}
		return h.receipts.AcknowledgeMessage(
			operationCtx,
			userID,
			deviceID,
			messageID,
		)
	case ActionRead:
		conversationID, err := strconv.ParseUint(message.ConversationID, 10, 64)
		if err != nil || conversationID == 0 || message.Seq == 0 {
			return fmt.Errorf("已读回执参数格式错误")
		}
		return h.receipts.MarkConversationRead(
			operationCtx,
			userID,
			conversationID,
			message.Seq,
		)
	default:
		return nil
	}
}

func (h *WebsocketHandler) updatePresence(
	ctx context.Context,
	userID uint64,
	deviceID uint64,
) {
	if h.presence == nil {
		return
	}
	presenceCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	if err := h.presence.Touch(presenceCtx, userID, deviceID); err != nil {
		slog.Warn("更新设备在线状态失败", "user_id", userID, "error", err)
	}
}

func (h *WebsocketHandler) removePresence(userID uint64, deviceID uint64) {
	if h.presence == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := h.presence.Remove(ctx, userID, deviceID); err != nil {
		slog.Warn("清理设备在线状态失败", "user_id", userID, "error", err)
	}
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
