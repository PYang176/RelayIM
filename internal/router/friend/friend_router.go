package friend

import (
	"ginChatProject/internal/handler"
	"ginChatProject/internal/repository"
	"ginChatProject/internal/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Register 注册好友模块的路由
// 前置域名为/friends
func Register(r *gin.RouterGroup, db *gorm.DB) {
	friendRepo := repository.NewFriendRepository(db)
	friendService := service.NewFriendService(friendRepo)
	friendHandler := handler.NewFriendHandler(friendService)

	r.POST("/request", friendHandler.SendRequest)
	r.GET("/requests/received", friendHandler.GetReceiveRequest)
	r.GET("/requests/sent", friendHandler.GetSendRequest)
	r.POST("/accept/:id", friendHandler.AcceptRequest)
	r.POST("/reject/:id", friendHandler.RejectRequest)
	r.DELETE("", friendHandler.DeleteFriend)
	r.GET("", friendHandler.ListFriends)
}
