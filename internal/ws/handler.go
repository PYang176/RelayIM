package ws

import (
	"fmt"
	"ginChatProject/utils/jwt"
	"ginChatProject/utils/redis_util"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"net/http"
	"time"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true //允许跨域，仅限开发环境
	},
}

// Connect websocket 连接入口
func Connect(c *gin.Context) {
	// 从URL中获取token
	tokenStr := c.Query("token")
	if tokenStr == "" {
		c.JSON(http.StatusUnauthorized, "未提供 token")
		return
	}

	// 如果token不为空，则对其进行解析校验
	claims, err := jwt.ParseToken(tokenStr)
	if err != nil {
		c.JSON(http.StatusForbidden, "token 无效或已过期")
		return
	}

	// 从Redis里面查token进行比对
	key := fmt.Sprintf("token:%s", tokenStr)
	val, err := redis_util.RDB.Get(c, key).Result()
	if err != nil || val == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "token 已经过期"})
		return
	}

	// 从token中拿到userID
	userID := claims.UserID
	fmt.Println(" WS connected:", userID)

	// 将http升级为websocket连接
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		fmt.Println("upgrade error:", err)
		return
	}
	defer conn.Close()

	// 保存到 ConnManager
	Manager.Add(userID, conn)
	defer Manager.Remove(userID)

	// 保持连接
	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			fmt.Println("ws closed:", err)
			break
		}
		time.Sleep(time.Second)
	}

}
