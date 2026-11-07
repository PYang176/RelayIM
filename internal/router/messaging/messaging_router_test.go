package messaging_test

import (
	"ginChatProject/internal/router/conversation"
	messagingrouter "ginChatProject/internal/router/messaging"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRegisterRoutesAlongsideConversationRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	// 两个模块共享 /api/v1/conversations 前缀，测试静态群路由与参数消息
	// 路由可以同时注册，避免启动时发生 Gin 路由冲突。
	conversation.Register(router, nil)
	messagingrouter.Register(router, nil)

	wantRoutes := map[string]bool{
		"POST /api/v1/conversations/groups":                                        false,
		"GET /api/v1/conversations/:conversation_id/read-states":                   false,
		"GET /api/v1/conversations/:conversation_id/messages":                      false,
		"GET /api/v1/conversations/:conversation_id/messages/:message_id/delivery": false,
		"POST /api/v1/conversations/:conversation_id/messages":                     false,
	}
	for _, route := range router.Routes() {
		key := route.Method + " " + route.Path
		if _, exists := wantRoutes[key]; exists {
			wantRoutes[key] = true
		}
	}
	for route, registered := range wantRoutes {
		if !registered {
			t.Fatalf("路由 %s 未注册", route)
		}
	}
}
