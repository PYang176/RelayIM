package ws

import (
	"context"
	"encoding/json"
	"fmt"
	"ginChatProject/internal/service"
	"ginChatProject/utils/jwt"
	"ginChatProject/utils/redis_util"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"log"
	"net/http"
	"time"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true //允许跨域，仅限开发环境
	},
}

type WebsocketHandler struct {
	msgService *service.MessageService
}

// NewWebsocketHandler 注入messageService
func NewWebsocketHandler(svc *service.MessageService) *WebsocketHandler {
	return &WebsocketHandler{msgService: svc}
}

// Connect websocket 连接入口
func (h *WebsocketHandler) Connect(c *gin.Context) {
	// 从URL中获取token
	tokenStr := c.Query("token")
	if tokenStr == "" {
		c.JSON(http.StatusUnauthorized, "未提供 token")
		return
	}

	// 如果token不为空，则对其进行解析校验
	claims, err := jwt.ParseToken(tokenStr)
	if err != nil {
		c.JSON(http.StatusForbidden, "token 无效或已过期")
		return
	}

	// 从Redis里面查token进行比对
	key := fmt.Sprintf("token:%s", tokenStr)
	val, err := redis_util.RDB.Get(c, key).Result()
	if err != nil || val == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "token 已经过期"})
		return
	}

	// 从token中拿到userID
	userID := claims.UserID
	fmt.Println(" WS connected:", userID)

	// 将http升级为websocket连接
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		fmt.Println("upgrade error:", err)
		return
	}
	defer conn.Close()

	// 保存到 ConnManager
	Manager.Add(userID, conn)
	defer Manager.Remove(userID)

	// 用户上线后自动推送离线消息
	go func(uid uint) {
		// 记录出错时候的堆栈信息
		defer func() {
			fmt.Println("卡在这里了！！！---------")
			if r := recover(); r != nil {
				log.Printf("推送离线消息 panic: %v\n", r)
			}
		}()

		messages, err := h.msgService.PullOfflineMessages(context.Background(), uid)
		if err != nil {
			fmt.Println("ws 拉取离线消息出错了:", err)
			return
		}
		// 遍历消息集进行消息发送
		for _, message := range messages {
			// 检查连接是否仍然有效
			if !Manager.IsOnline(uid) {
				return
			}

			data, _ := json.Marshal(WsMessage{
				MessageID:  message.MessageID,
				SenderID:   message.SenderID,
				ReceiverID: message.ReceiverID,
				MsgType:    int(message.MsgType),
				Content:    message.Content,
				URL:        message.URL,
				Action:     ActionMessage,
			})
			Manager.Send(uid, data)
		}
	}(userID)

	// 保持连接
	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			fmt.Println("ws closed:", err)
			break
		}

		time.Sleep(time.Second)

		var msg WsMessage
		// 将接收到的消息绑定到msg上
		if err := json.Unmarshal(raw, &msg); err != nil {
			continue
		}

		fmt.Println("服务端收到的客户端ack消息:", msg)

		switch msg.Action {
		// 接收客户端确认已送达
		case ActionAck:
			go func() {
				err := h.msgService.AckMessage(context.Background(), msg.MessageID, userID)
				if err != nil {
					fmt.Println("ack出错了：", err)
				}
			}()

		// 接收客户端确认已读
		case ActionRead:
			go func() {
				err := h.msgService.ReadMessage(context.Background(), msg.MessageID, userID)
				if err != nil {
					fmt.Println("read出错了：", err)
				}
			}()
		}
	}

}
