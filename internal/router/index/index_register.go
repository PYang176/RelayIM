package index

import (
	"github.com/PYang176/RelayIM/internal/api"
	"github.com/gin-gonic/gin"
)

// Register 注册
func Register(r *gin.Engine) {
	indexHandler := api.NewIndexHandler()
	r.GET("/", indexHandler.Index)
}
