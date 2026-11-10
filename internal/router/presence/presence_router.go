package presence

import (
	"github.com/PYang176/RelayIM/internal/middleware"
	"github.com/PYang176/RelayIM/internal/platform/observability"
	presenceapp "github.com/PYang176/RelayIM/internal/presence"
	"time"

	"github.com/gin-gonic/gin"
)

var queryRateLimitPolicy = middleware.RateLimitPolicy{
	Namespace:    "presence_query",
	Subject:      middleware.RateLimitByUser,
	SubjectLimit: 60,
	GlobalLimit:  1000,
	Window:       time.Minute,
}

// Register 注册在线状态查询路由。
func Register(
	r *gin.Engine,
	querier presenceapp.Querier,
	requestLimiter middleware.RateLimitChecker,
	metrics *observability.Metrics,
) {
	handler := presenceapp.NewHandler(querier)
	group := r.Group("/api/v1/presence")
	group.Use(middleware.AuthMiddleware())
	group.POST(
		"/query",
		middleware.RateLimit(requestLimiter, queryRateLimitPolicy, metrics),
		handler.Query,
	)
}
