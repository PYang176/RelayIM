package group

import (
	"ginChatProject/internal/api"
	"ginChatProject/internal/middleware"
	"ginChatProject/internal/repository"
	"ginChatProject/internal/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func Register(r *gin.Engine, db *gorm.DB) {
	groupRepo := repository.NewGroupRepository(db)
	groupService := service.NewGroupService(groupRepo)
	groupHandler := api.NewGroupHandler(groupService)

	groups := r.Group("/groups")
	groups.Use(middleware.AuthMiddleware())
	{
		// 创建群聊
		groups.POST("/create", groupHandler.CreateGroup)

		// 加入群聊
		groups.POST("/:id/join", groupHandler.JoinGroup)

		// 退出群聊
		groups.POST("/:id/leave", groupHandler.LeaveGroup)

		// 获取我的群列表
		groups.GET("/my", groupHandler.GetMyGroups)

		// 获取群成员列表
		groups.GET("/:id/members", groupHandler.GetGroupMembers)
	}
}
