package ws

import (
	"ginChatProject/internal/ws"
	"github.com/gin-gonic/gin"
)

// Register 注册新版 WebSocket 路由。
func Register(r *gin.Engine) {
	wsHandler := ws.NewWebsocketHandler()
	r.GET("/ws", wsHandler.Connect)
}
