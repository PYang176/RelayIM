package conversation

import (
	"errors"
	"ginChatProject/internal/middleware"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// Handler 处理会话 HTTP 请求。
type Handler struct {
	service *Service
}

// NewHandler 创建会话 Handler。
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

type createDirectRequest struct {
	// PeerUserID 是聊天对方 Snowflake ID 字符串。
	PeerUserID string `json:"peer_user_id"`
}

type createGroupRequest struct {
	// Name 是群名称。
	Name string `json:"name"`

	// AvatarURL 是可选群头像地址。
	AvatarURL string `json:"avatar_url"`
}

// CreateDirect 处理 POST /api/v1/conversations/private。
func (h *Handler) CreateDirect(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	var request createDirectRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数格式错误"})
		return
	}
	peerUserID, err := strconv.ParseUint(request.PeerUserID, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "peer_user_id 格式错误"})
		return
	}

	result, created, err := h.service.CreateDirect(
		c.Request.Context(),
		CreateDirectCommand{
			UserID:     userID,
			PeerUserID: peerUserID,
		},
	)
	if err != nil {
		writeConversationError(c, err)
		return
	}

	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	c.JSON(status, conversationResponse(result))
}

// CreateGroup 处理 POST /api/v1/conversations/groups。
func (h *Handler) CreateGroup(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	var request createGroupRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数格式错误"})
		return
	}

	result, err := h.service.CreateGroup(
		c.Request.Context(),
		CreateGroupCommand{
			OwnerID:   userID,
			Name:      request.Name,
			AvatarURL: request.AvatarURL,
		},
	)
	if err != nil {
		writeConversationError(c, err)
		return
	}

	c.JSON(http.StatusCreated, conversationResponse(result))
}

// List 处理 GET /api/v1/conversations。
func (h *Handler) List(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	results, err := h.service.List(c.Request.Context(), userID)
	if err != nil {
		writeConversationError(c, err)
		return
	}

	response := make([]gin.H, 0, len(results))
	for _, result := range results {
		response = append(response, conversationResponse(result))
	}
	c.JSON(http.StatusOK, gin.H{"conversations": response})
}

func currentUserID(c *gin.Context) (uint64, bool) {
	userID, ok := middleware.CurrentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "登录用户信息无效"})
		return 0, false
	}
	return userID, true
}

func conversationResponse(result Result) gin.H {
	return gin.H{
		"id":                strconv.FormatUint(result.ID, 10),
		"conversation_type": result.ConversationType,
		"name":              result.Name,
		"avatar_url":        result.AvatarURL,
		"last_seq":          result.LastSeq,
		"last_read_seq":     result.LastReadSeq,
		"unread_count":      result.UnreadCount,
	}
}

func writeConversationError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrInvalidInput), errors.Is(err, ErrSameUser):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, ErrUserNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": ErrUserNotFound.Error()})
	case errors.Is(err, ErrConversationNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": ErrConversationNotFound.Error()})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "服务器内部错误"})
	}
}
