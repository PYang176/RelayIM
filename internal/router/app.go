package router

import (
	messagingapp "ginChatProject/internal/messaging"
	"ginChatProject/internal/middleware"
	"ginChatProject/internal/platform/observability"
	presenceapp "ginChatProject/internal/presence"
	conversationrouter "ginChatProject/internal/router/conversation"
	friendshiprouter "ginChatProject/internal/router/friendship"
	identityrouter "ginChatProject/internal/router/identity"
	"ginChatProject/internal/router/index"
	messagingrouter "ginChatProject/internal/router/messaging"
	presencerouter "ginChatProject/internal/router/presence"
	"ginChatProject/internal/router/ws"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

//func Router() *gin.Engine {
//	r := gin.Default()
//	r.GET("/index", service.GetIndex)
//	return r
//}

func Router(
	db *gorm.DB,
	storage messagingapp.AttachmentStorage,
	metrics *observability.Metrics,
	health *observability.Health,
	requestLimiter middleware.RateLimitChecker,
	wsTicketLimiter middleware.WSTicketRateLimiter,
	presenceService *presenceapp.Service,
) *gin.Engine {
	r := gin.New()

	// 当前服务直接暴露给客户端。关闭代理头信任，避免伪造 X-Forwarded-For
	// 绕过按 IP 限流；接入可信反向代理后再显式配置代理网段。
	r.ForwardedByClientIP = false
	r.Use(
		middleware.ObserveRequest(metrics),
		gin.Recovery(),
	)

	if health != nil {
		r.GET("/health/live", gin.WrapH(health.LiveHandler()))
		r.GET("/health/ready", gin.WrapH(health.ReadyHandler()))
	}
	if metrics != nil {
		r.GET("/metrics", gin.WrapH(metrics.Handler()))
	}

	// 调试接口文档
	//docs.SwaggerInfo.BasePath = ""
	//r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// 新版用户认证模块
	identityrouter.Register(
		r,
		db,
		wsTicketLimiter,
		requestLimiter,
		metrics,
	)

	// 新版会话模块
	conversationrouter.Register(r, db, requestLimiter, metrics)

	// 新版好友模块
	friendshiprouter.Register(r, db, requestLimiter, metrics)

	// 新版消息模块
	messagingrouter.Register(r, db, storage, requestLimiter, metrics)

	// websocket
	ws.Register(r, db, presenceService)

	// 在线状态
	presencerouter.Register(r, presenceService, requestLimiter, metrics)

	// index
	index.Register(r)
	return r
}
