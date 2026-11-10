package middleware

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/PYang176/RelayIM/internal/config"
	"github.com/PYang176/RelayIM/internal/platform/jwt"
	"github.com/PYang176/RelayIM/internal/platform/redisstore"
	"github.com/gin-gonic/gin"
)

// TestAuthMiddlewareIntegration 覆盖认证中间件对有效、缺失和已撤销凭证的处理
func TestAuthMiddlewareIntegration(t *testing.T) {
	redisAddr := os.Getenv("RELAYIM_TEST_REDIS_ADDR")
	if redisAddr == "" {
		t.Skip("未设置 RELAYIM_TEST_REDIS_ADDR，跳过认证中间件集成测试")
	}

	redisHost, redisPortText, err := net.SplitHostPort(redisAddr)
	if err != nil {
		t.Fatalf("测试 Redis 地址必须为 host:port: %v", err)
	}
	redisPort, err := strconv.Atoi(redisPortText)
	if err != nil {
		t.Fatalf("解析测试 Redis 端口失败: %v", err)
	}
	if err := redisstore.InitRedis(config.RedisConfig{
		Host:     redisHost,
		Port:     redisPort,
		Password: os.Getenv("RELAYIM_TEST_REDIS_PASSWORD"),
		DB:       0,
	}); err != nil {
		t.Fatalf("连接测试 Redis 失败: %v", err)
	}

	oldJWTConfig := jwt.JwtCfg
	jwt.JwtCfg = config.JWTConfig{
		Secret:      "middleware-integration-secret",
		ExpireHours: 1,
	}
	t.Cleanup(func() {
		jwt.JwtCfg = oldJWTConfig
	})

	const userID = uint64(42)
	token, err := jwt.GenerateToken(userID, jwt.JwtCfg.ExpireHours)
	if err != nil {
		t.Fatalf("生成测试 Token 失败: %v", err)
	}
	claims, err := jwt.ParseToken(token)
	if err != nil {
		t.Fatalf("解析测试 Token 失败: %v", err)
	}

	tokenKey := "token:" + token
	blacklistKey := "blacklist:" + claims.JTI
	t.Cleanup(func() {
		redisstore.RDB.Del(context.Background(), tokenKey, blacklistKey)
	})

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(AuthMiddleware())
	router.GET("/private", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"user_id": c.GetUint64(ContextUserIDKey),
		})
	})

	// 没有 Authorization Header 时必须在进入业务 Handler 前中止请求。
	response := performAuthRequest(router, "")
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("无 Token 状态码 = %d，期望 401", response.Code)
	}

	// 合法 Bearer Token 需要同时存在于 Redis 白名单。
	if err := redisstore.RDB.Set(context.Background(), tokenKey, userID, time.Minute).Err(); err != nil {
		t.Fatalf("写入 Token 白名单失败: %v", err)
	}
	response = performAuthRequest(router, "Bearer "+token)
	if response.Code != http.StatusOK {
		t.Fatalf("合法 Token 状态码 = %d，期望 200", response.Code)
	}

	// 黑名单优先于白名单，用于退出登录后的立即撤销。
	if err := redisstore.RDB.Set(context.Background(), blacklistKey, 1, time.Minute).Err(); err != nil {
		t.Fatalf("写入黑名单失败: %v", err)
	}
	response = performAuthRequest(router, "Bearer "+token)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("黑名单 Token 状态码 = %d，期望 401", response.Code)
	}
	if err := redisstore.RDB.Del(context.Background(), blacklistKey).Err(); err != nil {
		t.Fatalf("清理测试黑名单失败: %v", err)
	}

	// 删除白名单后，签名仍然有效但会话已撤销。
	if err := redisstore.RDB.Del(context.Background(), tokenKey).Err(); err != nil {
		t.Fatalf("删除 Token 白名单失败: %v", err)
	}
	response = performAuthRequest(router, "Bearer "+token)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("白名单缺失状态码 = %d，期望 401", response.Code)
	}
}

// performAuthRequest 发起认证集成测试 HTTP 请求并返回响应记录器
func performAuthRequest(router http.Handler, authorization string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, "/private", nil)
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

// Example_authorizationToken 展示认证请求处理的标准用法
func Example_authorizationToken() {
	fmt.Println(authorizationToken("Bearer example-token"))
	// Output: example-token
}
