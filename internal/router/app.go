package router

import (
	messagingapp "ginChatProject/internal/messaging"
	"ginChatProject/internal/middleware"
	"ginChatProject/internal/platform/observability"
	conversationrouter "ginChatProject/internal/router/conversation"
	friendshiprouter "ginChatProject/internal/router/friendship"
	identityrouter "ginChatProject/internal/router/identity"
	"ginChatProject/internal/router/index"
	messagingrouter "ginChatProject/internal/router/messaging"
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
) *gin.Engine {
	r := gin.New()
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
	identityrouter.Register(r, db)

	// 新版会话模块
	conversationrouter.Register(r, db)

	// 新版好友模块
	friendshiprouter.Register(r, db)

	// 新版消息模块
	messagingrouter.Register(r, db, storage, metrics)

	// websocket
	ws.Register(r, db)

	// index
	index.Register(r)
	return r
}
