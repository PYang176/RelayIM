package ws

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	messagingapp "relayim/internal/messaging"
	"relayim/internal/ws"
)

// Register 注册 WebSocket 路由。
func Register(
	r *gin.Engine,
	db *gorm.DB,
	presence ws.PresenceTracker,
	routes ws.RouteTracker,
) {
	repository := messagingapp.NewRepository(db)
	service := messagingapp.NewService(repository)
	wsHandler := ws.NewWebsocketHandler(service, presence, routes)
	r.GET("/ws", wsHandler.Connect)
}
