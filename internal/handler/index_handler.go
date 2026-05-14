package handler

import "github.com/gin-gonic/gin"

type IndexHandler struct{}

func NewIndexHandler() *IndexHandler {
	return &IndexHandler{}
}

// Index
// @Summary 首页欢迎信息
// @Description 返回系统欢迎语
// @Tags index
// @Produce json
// @Success 200 {string} welcome
// @Router / [get]
func (h *IndexHandler) Index(c *gin.Context) {
	c.JSON(200, gin.H{
		"message": "welcome",
	})
}

//注释	作用
//@Summary	接口的简要说明
//@Description	接口的详细描述
//@Tags	接口分组（Swagger UI 左侧菜单）
//@Produce	返回的数据类型
//@Success	成功返回的结构
//@Router	路由地址和 HTTP 方法
