package user

import (
	"ginChatProject/internal/handler"
	"ginChatProject/internal/repository"
	"ginChatProject/internal/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Register 前置域名"/users"
func Register(r *gin.RouterGroup, db *gorm.DB) {
	repo := repository.NewUserRepository(db)
	svc := service.NewUserService(repo)
	h := handler.NewUserHandler(svc)

	r.GET("", h.List)
	r.DELETE("/:id", h.Delete)
	r.PUT("/:id", h.Update)
}
