package identity

import (
	identityapp "ginChatProject/internal/identity"
	"ginChatProject/internal/middleware"
	"ginChatProject/internal/platform/authn"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Register 注册新版用户认证路由。
func Register(r *gin.Engine, db *gorm.DB) {
	// 新认证链路只依赖新 ginchat_v2 用户表，不与旧 user_basic 混用。
	repo := identityapp.NewRepository(db)
	identityService := identityapp.NewService(repo, authn.DefaultManager)
	handler := identityapp.NewHandler(identityService)

	auth := r.Group("/api/v1/auth")
	auth.POST("/register", handler.Register)
	auth.POST("/login", handler.Login)

	protected := r.Group("/api/v1")
	protected.Use(middleware.AuthMiddleware())
	protected.GET("/me", handler.Me)
}
