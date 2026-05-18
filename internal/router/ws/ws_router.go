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
	friendRepo := repository.NewFriendRepository(db)
	msgRepo := repository.NewMessageRepository(db)
	groupRepo := repository.NewGroupRepository(db)

	msgService := service.NewMessageService(msgRepo, friendRepo, groupRepo)
	wsHandler := ws.NewWebsocketHandler(msgService)
	r.GET("/ws", wsHandler.Connect)

}
