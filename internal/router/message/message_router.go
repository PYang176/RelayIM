package message

import (
	"ginChatProject/internal/handler"
	"ginChatProject/internal/repository"
	"ginChatProject/internal/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Register 前置域名"/messages"
func Register(r *gin.RouterGroup, db *gorm.DB) {
	msgRepo := repository.NewMessageRepository(db)
	msgService := service.NewMessageService(msgRepo)
	msgHandler := handler.NewMessageHandler(msgService)

	r.POST("", msgHandler.Send)
	r.GET("", msgHandler.Count)

}
