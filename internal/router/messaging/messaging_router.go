package messaging

import (
	messagingapp "ginChatProject/internal/messaging"
	"ginChatProject/internal/middleware"
	"ginChatProject/internal/platform/observability"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Register 注册新版消息路由。
func Register(
	r *gin.Engine,
	db *gorm.DB,
	storage messagingapp.AttachmentStorage,
	metrics *observability.Metrics,
) {
	repository := messagingapp.NewRepository(db)
	service := messagingapp.NewServiceWithStorage(repository, storage)
	handler := messagingapp.NewHandlerWithMetrics(
		service,
		service,
		service,
		metrics,
	)

	group := r.Group("/api/v1/conversations")
	group.Use(middleware.AuthMiddleware())
	group.GET(
		"/:conversation_id/messages/:message_id/delivery",
		handler.GetDeliverySummary,
	)
	group.GET("/:conversation_id/read-states", handler.ListReadStates)
	group.GET("/:conversation_id/messages", handler.ListMessages)
	group.POST("/:conversation_id/messages", handler.SendMessage)

	attachments := r.Group("/api/v1/attachments")
	attachments.Use(middleware.AuthMiddleware())
	attachments.POST("", handler.UploadAttachment)
	attachments.GET("/:id", handler.GetAttachmentInfo)
	attachments.GET("/:id/content", handler.DownloadAttachment)
}
