package conversation

import (
	conversationapp "ginChatProject/internal/conversation"
	"ginChatProject/internal/middleware"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Register 注册新版会话路由。
func Register(r *gin.Engine, db *gorm.DB) {
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
}
