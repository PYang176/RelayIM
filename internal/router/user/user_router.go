package user

import (
	"ginChatProject/internal/handler"
	"ginChatProject/internal/middleware"
	"ginChatProject/internal/repository"
	"ginChatProject/internal/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Register 注册user相关的路由
func Register(r *gin.Engine, db *gorm.DB) {

	repo := repository.NewUserRepository(db)
	svc := service.NewUserService(repo)
	h := handler.NewUserHandler(svc)
	//登录方法
	r.POST("/users/login", h.Login)
	//注册方法
	r.POST("/users", h.Create)

	//使用拦截器进行拦截
	userGroup := r.Group("/users")
	userGroup.Use(middleware.AuthMiddleware())
	{
		userGroup.GET("", h.List)
		userGroup.DELETE("/:id", h.Delete)
		userGroup.PUT("/:id", h.Update)
		userGroup.POST("/logout", h.Logout)
	}

}
