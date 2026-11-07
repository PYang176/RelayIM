package router

import (
	messagingapp "ginChatProject/internal/messaging"
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
) *gin.Engine {
	r := gin.New()
	// WebSocket 当前仍需查询参数携带 Token；跳过该路径访问日志，避免凭证落盘。
	r.Use(
		gin.LoggerWithConfig(gin.LoggerConfig{
			SkipPaths: []string{"/ws"},
		}),
		gin.Recovery(),
	)

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
	messagingrouter.Register(r, db, storage)

	// websocket
	ws.Register(r, db)

	// index
	index.Register(r)
	return r
}
