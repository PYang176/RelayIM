package ws

import (
	"ginChatProject/internal/repository"
	"ginChatProject/internal/service"
	"ginChatProject/internal/ws"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Register websocket路由注册函数
func Register(r *gin.Engine, db *gorm.DB) {
	// 在线状态
	onlineChecker := ws.Manager
	msgRepo := repository.NewMessageRepository(db)
	msgService := service.NewMessageService(msgRepo, onlineChecker)
	wsHandler := ws.NewWebsocketHandler(msgService)
	r.GET("/ws", wsHandler.Connect)

}
