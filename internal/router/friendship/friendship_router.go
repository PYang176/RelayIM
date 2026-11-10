package friendship

import (
	friendshipapp "relayim/internal/friendship"
	"relayim/internal/middleware"
	"relayim/internal/platform/observability"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

var friendRequestRateLimitPolicy = middleware.RateLimitPolicy{
	Namespace:    "friend_request_create",
	Subject:      middleware.RateLimitByUser,
	SubjectLimit: 10,
	GlobalLimit:  200,
	Window:       time.Minute,
}

// Register 注册好友路由。
func Register(
	r *gin.Engine,
	db *gorm.DB,
	requestLimiter middleware.RateLimitChecker,
	metrics *observability.Metrics,
) {
	repository := friendshipapp.NewRepository(db)
	service := friendshipapp.NewService(repository)
	handler := friendshipapp.NewHandler(service)

	group := r.Group("/api/v1")
	group.Use(middleware.AuthMiddleware())
	group.GET("/users/search", handler.SearchUsers)
	group.POST(
		"/friend-requests",
		middleware.RateLimit(requestLimiter, friendRequestRateLimitPolicy, metrics),
		handler.SendRequest,
	)
	group.GET("/friend-requests/received", handler.ListReceivedRequests)
	group.GET("/friend-requests/sent", handler.ListSentRequests)
	group.POST("/friend-requests/:id/accept", handler.AcceptRequest)
	group.POST("/friend-requests/:id/reject", handler.RejectRequest)
	group.GET("/friends", handler.ListFriends)
	group.GET("/user-blocks", handler.ListBlockedUsers)
	group.POST("/users/:user_id/block", handler.BlockUser)
	group.DELETE("/users/:user_id/block", handler.UnblockUser)
	group.PUT("/friends/:user_id/remark", handler.SetRemark)
	group.DELETE("/friends/:user_id", handler.DeleteFriend)
}
