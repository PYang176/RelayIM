package handler

import (
	"ginChatProject/internal/dto"
	"ginChatProject/internal/service"
	"github.com/gin-gonic/gin"
	"net/http"
)

type FriendHandler struct {
	friendService *service.FriendService
}

func NewFriendHandler(friendService *service.FriendService) *FriendHandler {
	return &FriendHandler{friendService: friendService}
}

// SendRequest 发送好友申请接口
func (h *FriendHandler) SendRequest(c *gin.Context) {
	//  从 token 获取user_id
	fromUserID := c.MustGet("user_id").(uint)

	var req dto.FriendRequestDTO

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	//toUserIDStr := c.PostForm("to_user_id")
	//remark := c.PostForm("remark")

	//toUserID, err := strconv.ParseUint(toUserIDStr, 10, 64)
	//if err != nil {
	//	c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
	//	return
	//}

	err := h.friendService.SendRequest(c.Request.Context(), uint64(fromUserID), req.ToUserID, req.Remark)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "申请已发送"})
}
