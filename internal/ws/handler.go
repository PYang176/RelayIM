package ws

import (
	"context"
	"encoding/json"
	"ginChatProject/internal/platform/authn"
	"ginChatProject/internal/platform/redisstore"
	"ginChatProject/internal/service"
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
type WebsocketHandler struct {
	msgService *service.MessageService
}

// NewWebsocketHandler 注入消息服务。
func NewWebsocketHandler(svc *service.MessageService) *WebsocketHandler {
	return &WebsocketHandler{msgService: svc}
}

// Connect 完成鉴权后把连接交给单写协程客户端。
func (h *WebsocketHandler) Connect(c *gin.Context) {
	tokenStr := c.Query("token")
	if tokenStr == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未提供 token"})
		return
	}

	runtimeSession, err := authn.DefaultManager.Authenticate(
		c.Request.Context(),
		tokenStr,
	)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "token 无效或已过期"})
		return
	}

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("升级 WebSocket 连接失败: %v", err)
		return
	}

	client := NewClient(
		runtimeSession.UserID,
		conn,
		func(heartbeatCtx context.Context) {
			h.updateHeartbeat(heartbeatCtx, runtimeSession.UserID)
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

	// 离线消息加载拥有独立生命周期；连接关闭后 Enqueue 返回 false 并结束推送。
	go h.pushOfflineMessages(ctx, client, runtimeSession.UserID)

	client.Run(ctx, func(messageCtx context.Context, raw []byte) error {
		return h.handleClientMessage(messageCtx, runtimeSession.UserID, raw)
	})
}

func (h *WebsocketHandler) handleClientMessage(
	ctx context.Context,
	userID uint64,
	raw []byte,
) error {
	var message WsMessage
	if err := json.Unmarshal(raw, &message); err != nil {
		return nil
	}

	operationCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	switch message.Action {
	case ActionAck:
		if err := h.msgService.AckMessage(
			operationCtx,
			message.MessageID,
			uint(userID),
		); err != nil {
			return err
		}
	case ActionRead:
		if err := h.msgService.ReadMessage(
			operationCtx,
			message.MessageID,
			uint(userID),
		); err != nil {
			return err
		}
	}
	return nil
}

func (h *WebsocketHandler) pushOfflineMessages(
	ctx context.Context,
	client *Client,
	userID uint64,
) {
	loadCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	messages, err := h.msgService.PullOfflineMessages(loadCtx, uint(userID))
	if err != nil {
		log.Printf("加载 WebSocket 离线消息失败: user_id=%d error=%v", userID, err)
		return
	}

	for _, message := range messages {
		data, err := json.Marshal(WsMessage{
			MessageID:  message.MessageID,
			SenderID:   message.SenderID,
			ReceiverID: message.ReceiverID,
			MsgType:    int(message.MsgType),
			Content:    message.Content,
			URL:        message.URL,
			Action:     ActionMessage,
		})
		if err != nil {
			log.Printf("序列化离线消息失败: %v", err)
			continue
		}
		if !client.Enqueue(data) {
			return
		}
	}
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
