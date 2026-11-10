package index

import (
	"github.com/gin-gonic/gin"
	"relayim/internal/api"
)

// Register 注册
func Register(r *gin.Engine) {
	indexHandler := api.NewIndexHandler()
	r.GET("/", indexHandler.Index)
}
