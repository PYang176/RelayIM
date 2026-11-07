package router

import (
	conversationrouter "ginChatProject/internal/router/conversation"
	"ginChatProject/internal/router/friend"
	"ginChatProject/internal/router/group"
	identityrouter "ginChatProject/internal/router/identity"
	"ginChatProject/internal/router/index"
	"ginChatProject/internal/router/message"
	messagingrouter "ginChatProject/internal/router/messaging"
	"ginChatProject/internal/router/user"
	"ginChatProject/internal/router/ws"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

//func Router() *gin.Engine {
//	r := gin.Default()
//	r.GET("/index", service.GetIndex)
//	return r
//}

func Router(db *gorm.DB) *gin.Engine {
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

	// 用户相关模块
	user.Register(r, db)

	// 新版用户认证模块
	identityrouter.Register(r, db)

	// 新版会话模块
	conversationrouter.Register(r, db)

	// 新版消息模块
	messagingrouter.Register(r, db)

	// 消息相关模块
	message.Register(r, db)

	// 好友相关模块
	friend.Register(r, db)

	// websocket
	ws.Register(r, db)

	// 群相关模块
	group.Register(r, db)

	// index
	index.Register(r)
	return r
}
