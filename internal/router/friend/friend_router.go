package friend

import (
	"ginChatProject/internal/handler"
	"ginChatProject/internal/middleware"
	"ginChatProject/internal/repository"
	"ginChatProject/internal/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Register 注册好友模块的路由
func Register(r *gin.Engine, db *gorm.DB) {
	friendRepo := repository.NewFriendRepository(db)
	friendService := service.NewFriendService(friendRepo)
	friendHandler := handler.NewFriendHandler(friendService)

	friendGroup := r.Group("/friends")
	friendGroup.Use(middleware.AuthMiddleware())
	{
		friendGroup.POST("/request", friendHandler.SendRequest)
		friendGroup.GET("/requests/received", friendHandler.GetReceiveRequest)
		friendGroup.GET("/requests/sent", friendHandler.GetSendRequest)
		friendGroup.POST("/accept/:id", friendHandler.AcceptRequest)
		friendGroup.POST("/reject/:id", friendHandler.RejectRequest)
		friendGroup.DELETE("", friendHandler.DeleteFriend)
		friendGroup.GET("", friendHandler.ListFriends)
	}
}
