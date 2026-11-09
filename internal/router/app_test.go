package router

import (
	"testing"

	"ginChatProject/internal/platform/observability"
	"github.com/gin-gonic/gin"
)

func TestRouterOnlyExposesVersionedApplicationAPI(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := Router(
		nil,
		nil,
		observability.NewMetrics(),
		observability.NewHealth(),
		nil,
		nil,
		nil,
		nil,
	)
	if engine.ForwardedByClientIP {
		t.Fatal("默认配置不应信任客户端伪造的代理 IP 请求头")
	}

	routes := make(map[string]bool)
	for _, route := range engine.Routes() {
		routes[route.Method+" "+route.Path] = true
	}

	required := []string{
		"POST /api/v1/auth/register",
		"POST /api/v1/auth/login",
		"POST /api/v1/auth/refresh",
		"POST /api/v1/auth/logout",
		"POST /api/v1/auth/ws-ticket",
		"PUT /api/v1/me",
		"POST /api/v1/presence/query",
		"GET /api/v1/users/search",
		"POST /api/v1/friend-requests",
		"GET /api/v1/friend-requests/received",
		"GET /api/v1/friend-requests/sent",
		"POST /api/v1/friend-requests/:id/accept",
		"POST /api/v1/friend-requests/:id/reject",
		"GET /api/v1/friends",
		"GET /api/v1/user-blocks",
		"POST /api/v1/users/:user_id/block",
		"DELETE /api/v1/users/:user_id/block",
		"PUT /api/v1/friends/:user_id/remark",
		"DELETE /api/v1/friends/:user_id",
		"GET /api/v1/conversations",
		"GET /api/v1/conversations/:conversation_id",
		"POST /api/v1/conversations/private",
		"POST /api/v1/conversations/groups",
		"GET /api/v1/conversations/:conversation_id/members",
		"POST /api/v1/conversations/:conversation_id/members",
		"GET /api/v1/conversations/:conversation_id/invitations",
		"DELETE /api/v1/conversations/:conversation_id/invitations/:invitation_id",
		"GET /api/v1/group-invitations",
		"POST /api/v1/group-invitations/:id/accept",
		"POST /api/v1/group-invitations/:id/reject",
		"DELETE /api/v1/conversations/:conversation_id/members/:user_id",
		"PUT /api/v1/conversations/:conversation_id/members/:user_id/role",
		"PUT /api/v1/conversations/:conversation_id/profile",
		"POST /api/v1/conversations/:conversation_id/transfer-owner",
		"POST /api/v1/conversations/:conversation_id/dissolve",
		"GET /api/v1/conversations/:conversation_id/read-states",
		"GET /api/v1/conversations/:conversation_id/messages",
		"GET /api/v1/conversations/:conversation_id/messages/:message_id/delivery",
		"POST /api/v1/conversations/:conversation_id/messages",
		"DELETE /api/v1/conversations/:conversation_id/messages/:message_id",
		"POST /api/v1/attachments",
		"GET /api/v1/attachments/:id",
		"GET /api/v1/attachments/:id/content",
		"GET /ws",
		"GET /health/live",
		"GET /health/ready",
		"GET /metrics",
	}
	for _, route := range required {
		if !routes[route] {
			t.Fatalf("缺少应用路由 %s", route)
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
			t.Fatalf("已下线路由仍被注册 %s", route)
		}
	}
}
