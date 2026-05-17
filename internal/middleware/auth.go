package middleware

import (
	"fmt"
	"ginChatProject/utils/jwt"
	"ginChatProject/utils/redis_util"
	"github.com/gin-gonic/gin"
	"net/http"
)

// AuthMiddleware JWT鉴权中间件
func AuthMiddleware() gin.HandlerFunc {
	return func(context *gin.Context) {
		// 从请求头中获取token
		tokenStr := context.GetHeader("Authorization")
		// 如果token为空，直接阻止接下来的请求
		if tokenStr == "" {
			context.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "未提供 token",
			})
			return
		}

		// token不为空，则解析token进行校验
		claims, err := jwt.ParseToken(tokenStr)
		fmt.Println(tokenStr)
		if claims != nil {
			fmt.Println("-----------------", claims.UserID, tokenStr)
		}
		if err != nil {
			context.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "token 无效或者已过期",
			})
			return
		}

		// 使用JTI作为Redis黑名单key
		blacklistKey := fmt.Sprintf("blacklist:%s", claims.JTI)
		// 拿到结果集
		exists, _ := redis_util.RDB.Exists(context, blacklistKey).Result()
		if exists > 0 {
			context.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "token已失效"})
			return
		}

		// 进行Redis校验
		key := fmt.Sprintf("token:%s", tokenStr)
		val, err := redis_util.RDB.Get(context, key).Result()
		if err != nil || val == "" {
			context.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "token 已失效，请重新登录",
			})
			return
		}

		//检验token携带的user_id是否与Redis缓存的一致
		if val != fmt.Sprintf("%d", claims.UserID) {
			context.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "token 不合法",
			})
			return
		}

		// 注入user_id
		context.Set("user_id", claims.UserID)
		context.Next()
	}
}
