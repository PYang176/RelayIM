package messaging

import (
	messagingapp "ginChatProject/internal/messaging"
	"ginChatProject/internal/middleware"
	"ginChatProject/internal/platform/observability"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

var (
	messageRateLimitPolicy = middleware.RateLimitPolicy{
		Namespace:    "message_send",
		Subject:      middleware.RateLimitByUser,
		SubjectLimit: 20,
		GlobalLimit:  500,
		Window:       10 * time.Second,
	}
	attachmentRateLimitPolicy = middleware.RateLimitPolicy{
		Namespace:    "attachment_upload",
		Subject:      middleware.RateLimitByUser,
		SubjectLimit: 10,
		GlobalLimit:  100,
		Window:       time.Minute,
	}
	messageRecallRateLimitPolicy = middleware.RateLimitPolicy{
		Namespace:    "message_recall",
		Subject:      middleware.RateLimitByUser,
		SubjectLimit: 20,
		GlobalLimit:  200,
		Window:       time.Minute,
	}
	messageSearchRateLimitPolicy = middleware.RateLimitPolicy{
		Namespace:    "message_search",
		Subject:      middleware.RateLimitByUser,
		SubjectLimit: 30,
		GlobalLimit:  300,
		Window:       time.Minute,
	}
)

// Register 注册消息路由。
func Register(
	r *gin.Engine,
	db *gorm.DB,
	storage messagingapp.AttachmentStorage,
	requestLimiter middleware.RateLimitChecker,
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
	group.GET(
		"/:conversation_id/messages/search",
		middleware.RateLimit(
			requestLimiter,
			messageSearchRateLimitPolicy,
			metrics,
		),
		handler.SearchMessages,
	)
	group.POST(
		"/:conversation_id/messages",
		middleware.RateLimit(requestLimiter, messageRateLimitPolicy, metrics),
		handler.SendMessage,
	)
	group.DELETE(
		"/:conversation_id/messages/:message_id",
		middleware.RateLimit(requestLimiter, messageRecallRateLimitPolicy, metrics),
		handler.RecallMessage,
	)

	attachments := r.Group("/api/v1/attachments")
	attachments.Use(middleware.AuthMiddleware())
	attachments.POST(
		"",
		middleware.RateLimit(requestLimiter, attachmentRateLimitPolicy, metrics),
		handler.UploadAttachment,
	)
	attachments.GET("/:id", handler.GetAttachmentInfo)
	attachments.GET("/:id/content", handler.DownloadAttachment)
}
