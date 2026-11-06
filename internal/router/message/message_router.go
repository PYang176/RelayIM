package message

import (
	"ginChatProject/internal/api"
	"ginChatProject/internal/middleware"
	"ginChatProject/internal/repository"
	"ginChatProject/internal/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Register 前置域名"/messages"
func Register(r *gin.Engine, db *gorm.DB) {
	// 在线状态
	friendRepo := repository.NewFriendRepository(db)
	groupRepo := repository.NewGroupRepository(db)
	msgRepo := repository.NewMessageRepository(db)
	msgService := service.NewMessageService(msgRepo, friendRepo, groupRepo)
	msgHandler := api.NewMessageHandler(msgService)

	msgGroup := r.Group("/messages")
	msgGroup.Use(middleware.AuthMiddleware())
	{
		msgGroup.POST("", msgHandler.Send)
		msgGroup.GET("", msgHandler.Count)
		// 群消息路由
		msgGroup.POST("/group/:id", msgHandler.SendGroupMessage)
	}

}
