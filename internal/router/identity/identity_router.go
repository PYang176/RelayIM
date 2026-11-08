package identity

import (
	identityapp "ginChatProject/internal/identity"
	"ginChatProject/internal/middleware"
	"ginChatProject/internal/platform/authn"
	"ginChatProject/internal/platform/observability"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

var (
	registerRateLimitPolicy = middleware.RateLimitPolicy{
		Namespace:    "auth_register",
		Subject:      middleware.RateLimitByIP,
		SubjectLimit: 5,
		GlobalLimit:  100,
		Window:       time.Minute,
	}
	loginRateLimitPolicy = middleware.RateLimitPolicy{
		Namespace:    "auth_login",
		Subject:      middleware.RateLimitByIP,
		SubjectLimit: 10,
		GlobalLimit:  200,
		Window:       time.Minute,
	}
	refreshRateLimitPolicy = middleware.RateLimitPolicy{
		Namespace:    "auth_refresh",
		Subject:      middleware.RateLimitByIP,
		SubjectLimit: 20,
		GlobalLimit:  300,
		Window:       time.Minute,
	}
)

// Register 注册新版用户认证路由。
func Register(
	r *gin.Engine,
	db *gorm.DB,
	wsTicketLimiter middleware.WSTicketRateLimiter,
	requestLimiter middleware.RateLimitChecker,
	metrics *observability.Metrics,
) {
	// 新认证链路只依赖新 ginchat_v2 用户表，不与旧 user_basic 混用。
	repo := identityapp.NewRepository(db)
	identityService := identityapp.NewService(repo, authn.DefaultManager)
	handler := identityapp.NewHandler(identityService, authn.DefaultManager)

	auth := r.Group("/api/v1/auth")
	auth.POST(
		"/register",
		middleware.RateLimit(requestLimiter, registerRateLimitPolicy, metrics),
		handler.Register,
	)
	auth.POST(
		"/login",
		middleware.RateLimit(requestLimiter, loginRateLimitPolicy, metrics),
		handler.Login,
	)
	auth.POST(
		"/refresh",
		middleware.RateLimit(requestLimiter, refreshRateLimitPolicy, metrics),
		handler.Refresh,
	)

	protected := r.Group("/api/v1")
	protected.Use(middleware.AuthMiddleware())
	protected.GET("/me", handler.Me)
	protected.POST("/auth/logout", handler.Logout)
	protected.POST(
		"/auth/ws-ticket",
		middleware.WSTicketRateLimit(wsTicketLimiter, metrics),
		handler.WSTicket,
	)
}
