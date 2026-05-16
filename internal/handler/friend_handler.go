package handler

import (
	"ginChatProject/internal/dto"
	"ginChatProject/internal/service"
	"github.com/gin-gonic/gin"
	"net/http"
	"strconv"
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

	// 获取请求参数JSON
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	err := h.friendService.SendRequest(c.Request.Context(), uint64(fromUserID), req.ToUserID, req.Remark)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "申请已发送"})
}

// AcceptRequest 接受好友申请
func (h *FriendHandler) AcceptRequest(c *gin.Context) {
	// 从URL路径中拿requestID
	requestIDStr := c.Param("id")
	requestID, err := strconv.ParseUint(requestIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}

	// token中拿userID
	userID := c.MustGet("user_id").(uint)

	// 调用Service函数接受好友申请
	err = h.friendService.AcceptRequest(c.Request.Context(), uint(requestID), uint64(userID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "已同意申请"})
}

// RejectRequest 拒绝好友申请
func (h *FriendHandler) RejectRequest(c *gin.Context) {
	// 从URL路径中拿requestID
	requestIDStr := c.Param("id")
	requestID, err := strconv.ParseUint(requestIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}

	// token中拿userID
	userID := c.MustGet("user_id").(uint)

	// 调用Service函数拒绝好友申请
	err = h.friendService.RejectRequest(c.Request.Context(), uint(requestID), uint64(userID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "已成功拒绝申请"})
}

// GetReceiveRequest 获取自己收到的好友申请
func (h *FriendHandler) GetReceiveRequest(c *gin.Context) {
	// token中拿userID
	userID := c.MustGet("user_id").(uint)

	request, err := h.friendService.ListReceiveFriendRequest(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, request)
}

// GetSendRequest 获取自己发送的好友申请
func (h *FriendHandler) GetSendRequest(c *gin.Context) {
	// token中拿userID
	userID := c.MustGet("user_id").(uint)

	request, err := h.friendService.ListSendFriendRequest(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, request)
}

// ListFriends 获取好友列表
func (h *FriendHandler) ListFriends(c *gin.Context) {
	// 从token中拿userID
	userID := c.MustGet("user_id").(uint)

	request, err := h.friendService.ListFriends(c.Request.Context(), uint64(userID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, request)
}

// DeleteFriend 删除好友
func (h *FriendHandler) DeleteFriend(c *gin.Context) {
	// token中拿userID
	userID := c.MustGet("user_id").(uint)

	var req struct {
		FriendID uint64 `json:"friend_id" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}

	err := h.friendService.DeleteFriend(
		c.Request.Context(),
		uint64(userID),
		req.FriendID,
	)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "好友已删除"})
}
