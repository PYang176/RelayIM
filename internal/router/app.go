package router

import (
	"ginChatProject/docs"
	"ginChatProject/internal/handler"
	"ginChatProject/internal/middleware"
	"ginChatProject/internal/repository"
	"ginChatProject/internal/router/friend"
	"ginChatProject/internal/router/index"
	"ginChatProject/internal/router/message"
	"ginChatProject/internal/router/user"
	"ginChatProject/internal/router/ws"
	"ginChatProject/internal/service"
	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"gorm.io/gorm"
)

//func Router() *gin.Engine {
//	r := gin.Default()
//	r.GET("/index", service.GetIndex)
//	return r
//}

func Router(db *gorm.DB) *gin.Engine {
	r := gin.Default()

	docs.SwaggerInfo.BasePath = ""
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	//登录方法
	repo := repository.NewUserRepository(db)
	svc := service.NewUserService(repo)
	h := handler.NewUserHandler(svc)
	r.POST("/users/login", h.Login)
	//注册方法
	r.POST("/users", h.Create)

	//调用注册函数注册路由users
	//使用拦截器进行拦截
	userGroup := r.Group("/users")
	userGroup.Use(middleware.AuthMiddleware())
	{
		user.Register(userGroup, db)
	}

	// 消息相关模块--拦截
	msgGroup := r.Group("/messages")
	msgGroup.Use(middleware.AuthMiddleware())
	{
		message.Register(msgGroup, db)
	}

	// 好友相关模块--拦截
	friendGroup := r.Group("/friends")
	friendGroup.Use(middleware.AuthMiddleware())
	{
		friend.Register(friendGroup, db)
	}

	// websocket
	ws.Register(r, db)

	// index
	index.Register(r)
	return r
}
