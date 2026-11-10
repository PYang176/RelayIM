package router

import (
	messagingapp "github.com/PYang176/RelayIM/internal/messaging"
	"github.com/PYang176/RelayIM/internal/middleware"
	"github.com/PYang176/RelayIM/internal/platform/observability"
	presenceapp "github.com/PYang176/RelayIM/internal/presence"
	realtimeapp "github.com/PYang176/RelayIM/internal/realtime"
	conversationrouter "github.com/PYang176/RelayIM/internal/router/conversation"
	friendshiprouter "github.com/PYang176/RelayIM/internal/router/friendship"
	identityrouter "github.com/PYang176/RelayIM/internal/router/identity"
	"github.com/PYang176/RelayIM/internal/router/index"
	messagingrouter "github.com/PYang176/RelayIM/internal/router/messaging"
	presencerouter "github.com/PYang176/RelayIM/internal/router/presence"
	"github.com/PYang176/RelayIM/internal/router/ws"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Router 计算r目标并完成路由
func Router(
	db *gorm.DB,
	storage messagingapp.AttachmentStorage,
	metrics *observability.Metrics,
	health *observability.Health,
	requestLimiter middleware.RateLimitChecker,
	wsTicketLimiter middleware.WSTicketRateLimiter,
	presenceService *presenceapp.Service,
	realtimeRoutes *realtimeapp.Registry,
) *gin.Engine {
	r := gin.New()

	// 当前服务直接暴露给客户端。关闭代理头信任，避免伪造 X-Forwarded-For
	// 绕过按 IP 限流；接入可信反向代理后再显式配置代理网段。
	r.ForwardedByClientIP = false
	r.Use(
		middleware.ObserveRequest(metrics),
		middleware.TraceRequest(),
		gin.Recovery(),
	)

	if health != nil {
		r.GET("/health/live", gin.WrapH(health.LiveHandler()))
		r.GET("/health/ready", gin.WrapH(health.ReadyHandler()))
	}
	if metrics != nil {
		r.GET("/metrics", gin.WrapH(metrics.Handler()))
	}

	// 用户认证路由
	identityrouter.Register(
		r,
		db,
		storage,
		wsTicketLimiter,
		requestLimiter,
		metrics,
	)

	// 会话路由
	conversationrouter.Register(r, db, requestLimiter, metrics)

	// 好友路由
	friendshiprouter.Register(r, db, requestLimiter, metrics)

	// 消息路由
	messagingrouter.Register(r, db, storage, requestLimiter, metrics)

	// websocket
	ws.Register(r, db, presenceService, realtimeRoutes)

	// 在线状态
	presencerouter.Register(r, presenceService, requestLimiter, metrics)

	// index
	index.Register(r)
	return r
}
