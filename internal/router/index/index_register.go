package index

import (
	"ginChatProject/internal/handler"
	"github.com/gin-gonic/gin"
)

func Register(r *gin.Engine) {
	indexHandler := handler.NewIndexHandler()
	r.GET("/", indexHandler.Index)
}

//func GetIndex(c *gin.Context) {
//	c.JSON(200, gin.H{
//		"message": "welcome",
//	})
//}
