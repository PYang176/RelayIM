package api

import "github.com/gin-gonic/gin"

type IndexHandler struct{}

func NewIndexHandler() *IndexHandler {
	return &IndexHandler{}
}

// Index 返回服务欢迎信息。
func (h *IndexHandler) Index(c *gin.Context) {
	c.JSON(200, gin.H{
		"message": "welcome",
	})
}
