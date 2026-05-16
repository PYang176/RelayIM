package message

import (
	"ginChatProject/internal/handler"
	"ginChatProject/internal/repository"
	"ginChatProject/internal/service"
	"ginChatProject/internal/ws"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Register 前置域名"/messages"
func Register(r *gin.RouterGroup, db *gorm.DB) {
	// 在线状态
	onlineChecker := ws.Manager
	friendRepo := repository.NewFriendRepository(db)

	msgRepo := repository.NewMessageRepository(db)
	msgService := service.NewMessageService(msgRepo, friendRepo, onlineChecker)
	msgHandler := handler.NewMessageHandler(msgService)

	r.POST("", msgHandler.Send)
	r.GET("", msgHandler.Count)

}
