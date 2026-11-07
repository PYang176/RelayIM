package router

import (
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRouterOnlyExposesVersionedApplicationAPI(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := Router(nil)

	routes := make(map[string]bool)
	for _, route := range engine.Routes() {
		routes[route.Method+" "+route.Path] = true
	}

	required := []string{
		"POST /api/v1/auth/register",
		"POST /api/v1/auth/login",
		"POST /api/v1/auth/ws-ticket",
		"GET /api/v1/conversations",
		"POST /api/v1/conversations/private",
		"GET /api/v1/conversations/:conversation_id/read-states",
		"GET /api/v1/conversations/:conversation_id/messages",
		"GET /api/v1/conversations/:conversation_id/messages/:message_id/delivery",
		"POST /api/v1/conversations/:conversation_id/messages",
		"GET /ws",
	}
	for _, route := range required {
		if !routes[route] {
			t.Fatalf("缺少新版路由 %s", route)
		}
	}

	forbidden := []string{
		"GET /users",
		"POST /users",
		"GET /messages",
		"POST /messages",
		"GET /friends",
		"POST /friends/request",
		"GET /groups/my",
		"POST /groups/create",
	}
	for _, route := range forbidden {
		if routes[route] {
			t.Fatalf("旧版路由仍被注册 %s", route)
		}
	}
}
