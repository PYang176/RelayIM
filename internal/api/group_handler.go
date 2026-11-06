package api

import (
	"ginChatProject/internal/dto"
	"ginChatProject/internal/service"
	"github.com/gin-gonic/gin"
	"net/http"
	"strconv"
)

type GroupHandler struct {
	groupService *service.GroupService
}

func NewGroupHandler(svc *service.GroupService) *GroupHandler {
	return &GroupHandler{groupService: svc}
}

// CreateGroup 创建群聊
func (h *GroupHandler) CreateGroup(c *gin.Context) {
	// 获取token中解析的userID
	userID := c.MustGet("user_id").(uint)

	var req dto.CreateGroupDTO

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	group, err := h.groupService.CreateGroup(
		c.Request.Context(),
		uint64(userID),
		req.Name,
		req.Avatar,
	)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "创建成功",
		"group":   group,
	})
}

// JoinGroup 加入群聊
func (h *GroupHandler) JoinGroup(c *gin.Context) {
	userID := c.MustGet("user_id").(uint)
	groupIDStr := c.Param("id")
	groupID, err := strconv.ParseUint(groupIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}

	var req struct {
		Nickname string `json:"nickname"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.groupService.JoinGroup(
		c.Request.Context(),
		groupID,
		uint64(userID),
		req.Nickname,
	); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "加入成功"})
}

// LeaveGroup 退出群聊
func (h *GroupHandler) LeaveGroup(c *gin.Context) {
	userID := c.MustGet("user_id").(uint)
	groupIDStr := c.Param("id")
	groupID, err := strconv.ParseUint(groupIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}

	if err := h.groupService.LeaveGroup(
		c.Request.Context(),
		groupID,
		uint64(userID),
	); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "退出成功"})
}

// GetMyGroups 获取我的群列表
func (h *GroupHandler) GetMyGroups(c *gin.Context) {
	userID := c.MustGet("user_id").(uint)

	groups, err := h.groupService.GetMyGroups(c.Request.Context(), uint64(userID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"groups": groups})
}

// GetGroupMembers 获取群成员列表
func (h *GroupHandler) GetGroupMembers(c *gin.Context) {
	groupIDStr := c.Param("id")
	groupID, err := strconv.ParseUint(groupIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}

	members, err := h.groupService.GetGroupMembers(c.Request.Context(), groupID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"members": members})
}
