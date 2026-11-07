package friendship

import (
	friendshipapp "ginChatProject/internal/friendship"
	"ginChatProject/internal/middleware"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Register 注册新版好友路由。
func Register(r *gin.Engine, db *gorm.DB) {
	repository := friendshipapp.NewRepository(db)
	service := friendshipapp.NewService(repository)
	handler := friendshipapp.NewHandler(service)

	group := r.Group("/api/v1")
	group.Use(middleware.AuthMiddleware())
	group.GET("/users/search", handler.SearchUsers)
	group.POST("/friend-requests", handler.SendRequest)
	group.GET("/friend-requests/received", handler.ListReceivedRequests)
	group.GET("/friend-requests/sent", handler.ListSentRequests)
	group.POST("/friend-requests/:id/accept", handler.AcceptRequest)
	group.POST("/friend-requests/:id/reject", handler.RejectRequest)
	group.GET("/friends", handler.ListFriends)
	group.PUT("/friends/:user_id/remark", handler.SetRemark)
	group.DELETE("/friends/:user_id", handler.DeleteFriend)
}
