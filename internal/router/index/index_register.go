package index

import (
	"ginChatProject/internal/api"
	"github.com/gin-gonic/gin"
)

func Register(r *gin.Engine) {
	indexHandler := api.NewIndexHandler()
	r.GET("/", indexHandler.Index)
}

//func GetIndex(c *gin.Context) {
//	c.JSON(200, gin.H{
//		"message": "welcome",
//	})
//}
