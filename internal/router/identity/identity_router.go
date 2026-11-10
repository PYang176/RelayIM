package identity

import (
	identityapp "relayim/internal/identity"
	"relayim/internal/middleware"
	"relayim/internal/platform/authn"
	"relayim/internal/platform/observability"
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
	profileUpdateRateLimitPolicy = middleware.RateLimitPolicy{
		Namespace:    "profile_update",
		Subject:      middleware.RateLimitByUser,
		SubjectLimit: 20,
		GlobalLimit:  200,
		Window:       time.Minute,
	}
	avatarUploadRateLimitPolicy = middleware.RateLimitPolicy{
		Namespace:    "profile_avatar_upload",
		Subject:      middleware.RateLimitByUser,
		SubjectLimit: 10,
		GlobalLimit:  100,
		Window:       time.Minute,
	}
)

// Register 注册用户认证路由。
func Register(
	r *gin.Engine,
	db *gorm.DB,
	avatars identityapp.AvatarStorage,
	wsTicketLimiter middleware.WSTicketRateLimiter,
	requestLimiter middleware.RateLimitChecker,
	metrics *observability.Metrics,
) {
	// 认证模块只依赖 users 和 user_profiles 表。
	repo := identityapp.NewRepository(db)
	identityService := identityapp.NewServiceWithAvatarStorage(
		repo,
		authn.DefaultManager,
		avatars,
	)
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
	protected.PUT(
		"/me",
		middleware.RateLimit(
			requestLimiter,
			profileUpdateRateLimitPolicy,
			metrics,
		),
		handler.UpdateProfile,
	)
	protected.POST(
		"/me/avatar",
		middleware.RateLimit(
			requestLimiter,
			avatarUploadRateLimitPolicy,
			metrics,
		),
		handler.UploadAvatar,
	)
	protected.DELETE("/me/avatar", handler.RemoveAvatar)
	protected.POST("/auth/logout", handler.Logout)
	protected.POST(
		"/auth/ws-ticket",
		middleware.WSTicketRateLimit(wsTicketLimiter, metrics),
		handler.WSTicket,
	)
	r.GET("/api/v1/users/:user_id/avatar", handler.OpenAvatar)
}
