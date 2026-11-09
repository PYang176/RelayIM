package conversation

import (
	conversationapp "ginChatProject/internal/conversation"
	"ginChatProject/internal/middleware"
	"ginChatProject/internal/platform/observability"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

var groupProfileRateLimitPolicy = middleware.RateLimitPolicy{
	Namespace:    "group_profile_update",
	Subject:      middleware.RateLimitByUser,
	SubjectLimit: 20,
	GlobalLimit:  200,
	Window:       time.Minute,
}

// Register 注册会话路由。
func Register(
	r *gin.Engine,
	db *gorm.DB,
	requestLimiter middleware.RateLimitChecker,
	metrics *observability.Metrics,
) {
	repo := conversationapp.NewRepository(db)
	service := conversationapp.NewService(repo)
	handler := conversationapp.NewHandler(service)

	group := r.Group("/api/v1/conversations")
	group.Use(middleware.AuthMiddleware())
	group.GET("", handler.List)
	group.POST("/private", handler.CreateDirect)
	group.POST("/groups", handler.CreateGroup)
	group.GET("/:conversation_id/members", handler.ListMembers)
	group.POST("/:conversation_id/members", handler.InviteMembers)
	group.DELETE("/:conversation_id/members/:user_id", handler.RemoveMember)
	group.PUT(
		"/:conversation_id/members/:user_id/role",
		handler.SetMemberRole,
	)
	group.POST("/:conversation_id/transfer-owner", handler.TransferOwner)
	group.POST("/:conversation_id/dissolve", handler.DissolveGroup)
	group.PUT(
		"/:conversation_id/profile",
		middleware.RateLimit(
			requestLimiter,
			groupProfileRateLimitPolicy,
			metrics,
		),
		handler.UpdateGroupProfile,
	)

	invitations := r.Group("/api/v1")
	invitations.Use(middleware.AuthMiddleware())
	invitations.GET("/group-invitations", handler.ListReceivedInvitations)
	invitations.POST("/group-invitations/:id/accept", handler.AcceptInvitation)
	invitations.POST("/group-invitations/:id/reject", handler.RejectInvitation)
	invitations.GET(
		"/conversations/:conversation_id/invitations",
		handler.ListGroupInvitations,
	)
}
