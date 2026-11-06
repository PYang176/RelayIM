package messaging

import (
	messagingapp "ginChatProject/internal/messaging"
	"ginChatProject/internal/middleware"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Register 注册新版消息路由。
func Register(r *gin.Engine, db *gorm.DB) {
	repository := messagingapp.NewRepository(db)
	service := messagingapp.NewService(repository)
	handler := messagingapp.NewHandler(service)

	group := r.Group("/api/v1/conversations")
	group.Use(middleware.AuthMiddleware())
	group.POST("/:conversation_id/messages", handler.SendText)
}
