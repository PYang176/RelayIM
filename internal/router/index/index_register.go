package index

import (
	"ginChatProject/internal/api"
	"github.com/gin-gonic/gin"
)

// Register 注册
func Register(r *gin.Engine) {
	indexHandler := api.NewIndexHandler()
	r.GET("/", indexHandler.Index)
}
