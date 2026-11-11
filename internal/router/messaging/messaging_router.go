package messaging

import (
	"time"

	"github.com/PYang176/RelayIM/internal/config"
	messagingapp "github.com/PYang176/RelayIM/internal/messaging"
	"github.com/PYang176/RelayIM/internal/middleware"
	"github.com/PYang176/RelayIM/internal/platform/observability"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

var (
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
	rateLimitConfig config.MessageRateLimitConfig,
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
		middleware.RateLimit(
			requestLimiter,
			messageSendRateLimitPolicy(rateLimitConfig),
			metrics,
		),
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

// messageSendRateLimitPolicy 把部署配置转换为消息发送限流策略
func messageSendRateLimitPolicy(
	cfg config.MessageRateLimitConfig,
) middleware.RateLimitPolicy {
	if cfg.SubjectLimit <= 0 {
		cfg.SubjectLimit = 20
	}
	if cfg.GlobalLimit <= 0 {
		cfg.GlobalLimit = 1000
	}
	if cfg.WindowMilliseconds <= 0 {
		cfg.WindowMilliseconds = 10000
	}
	return middleware.RateLimitPolicy{
		Namespace:    "message_send",
		Subject:      middleware.RateLimitByUser,
		SubjectLimit: cfg.SubjectLimit,
		GlobalLimit:  cfg.GlobalLimit,
		Window:       time.Duration(cfg.WindowMilliseconds) * time.Millisecond,
	}
}
