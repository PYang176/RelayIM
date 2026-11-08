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

	// MemberUserIDs 是创建群时一起加入的好友 Snowflake ID 字符串。
	MemberUserIDs []string `json:"member_user_ids"`
}

type inviteMembersRequest struct {
	// MemberUserIDs 是继续邀请入群的好友 Snowflake ID 字符串。
	MemberUserIDs []string `json:"member_user_ids"`
}

type setMemberRoleRequest struct {
	// Role 是目标角色，1 为普通成员、2 为管理员。
	Role int `json:"role"`
}

type transferOwnerRequest struct {
	// UserID 是接管群主身份的有效成员 Snowflake ID 字符串。
	UserID string `json:"user_id"`
}

type updateGroupProfileRequest struct {
	// Name 是新的群名称。
	Name string `json:"name"`

	// AvatarURL 是可选群头像地址，空字符串表示清除头像。
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
	memberUserIDs, ok := parseGroupMemberIDs(c, request.MemberUserIDs)
	if !ok {
		return
	}

	result, err := h.service.CreateGroup(
		c.Request.Context(),
		CreateGroupCommand{
			OwnerID:       userID,
			Name:          request.Name,
			AvatarURL:     request.AvatarURL,
			MemberUserIDs: memberUserIDs,
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

// ListMembers 处理 GET /api/v1/conversations/:conversation_id/members。
func (h *Handler) ListMembers(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	conversationID, err := strconv.ParseUint(c.Param("conversation_id"), 10, 64)
	if err != nil || conversationID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "conversation_id 格式错误"})
		return
	}
	results, err := h.service.ListMembers(
		c.Request.Context(),
		userID,
		conversationID,
	)
	if err != nil {
		writeConversationError(c, err)
		return
	}
	members := make([]gin.H, 0, len(results))
	for _, result := range results {
		members = append(members, gin.H{
			"user_id":    strconv.FormatUint(result.UserID, 10),
			"username":   result.Username,
			"nickname":   result.Nickname,
			"avatar_url": result.AvatarURL,
			"role":       result.Role,
			"joined_at":  result.JoinedAt,
		})
	}
	c.JSON(http.StatusOK, gin.H{"members": members})
}

// InviteMembers 处理 POST /api/v1/conversations/:conversation_id/members。
func (h *Handler) InviteMembers(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	conversationID, ok := parseConversationID(c)
	if !ok {
		return
	}
	var request inviteMembersRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数格式错误"})
		return
	}
	memberUserIDs, ok := parseGroupMemberIDs(c, request.MemberUserIDs)
	if !ok {
		return
	}
	memberCount, err := h.service.InviteMembers(
		c.Request.Context(),
		InviteMembersCommand{
			ActorID:        userID,
			ConversationID: conversationID,
			MemberUserIDs:  memberUserIDs,
		},
	)
	if err != nil {
		writeConversationError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"member_count": memberCount})
}

// RemoveMember 处理 DELETE /api/v1/conversations/:conversation_id/members/:user_id。
func (h *Handler) RemoveMember(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	conversationID, ok := parseConversationID(c)
	if !ok {
		return
	}
	targetUserID, err := strconv.ParseUint(c.Param("user_id"), 10, 64)
	if err != nil || targetUserID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user_id 格式错误"})
		return
	}
	if err := h.service.RemoveMember(
		c.Request.Context(),
		RemoveMemberCommand{
			ActorID:        userID,
			ConversationID: conversationID,
			TargetUserID:   targetUserID,
		},
	); err != nil {
		writeConversationError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// SetMemberRole 处理 PUT /api/v1/conversations/:conversation_id/members/:user_id/role。
func (h *Handler) SetMemberRole(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	conversationID, ok := parseConversationID(c)
	if !ok {
		return
	}
	targetUserID, ok := parsePathUserID(c, "user_id")
	if !ok {
		return
	}
	var request setMemberRoleRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数格式错误"})
		return
	}
	if err := h.service.SetMemberRole(
		c.Request.Context(),
		SetMemberRoleCommand{
			ActorID:        userID,
			ConversationID: conversationID,
			TargetUserID:   targetUserID,
			Role:           request.Role,
		},
	); err != nil {
		writeConversationError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// TransferOwner 处理 POST /api/v1/conversations/:conversation_id/transfer-owner。
func (h *Handler) TransferOwner(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	conversationID, ok := parseConversationID(c)
	if !ok {
		return
	}
	var request transferOwnerRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数格式错误"})
		return
	}
	targetUserID, err := strconv.ParseUint(request.UserID, 10, 64)
	if err != nil || targetUserID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user_id 格式错误"})
		return
	}
	if err := h.service.TransferOwner(
		c.Request.Context(),
		TransferOwnerCommand{
			ActorID:        userID,
			ConversationID: conversationID,
			TargetUserID:   targetUserID,
		},
	); err != nil {
		writeConversationError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// DissolveGroup 处理 POST /api/v1/conversations/:conversation_id/dissolve。
func (h *Handler) DissolveGroup(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	conversationID, ok := parseConversationID(c)
	if !ok {
		return
	}
	if err := h.service.DissolveGroup(
		c.Request.Context(),
		DissolveGroupCommand{
			ActorID:        userID,
			ConversationID: conversationID,
		},
	); err != nil {
		writeConversationError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// UpdateGroupProfile 处理 PUT /api/v1/conversations/:conversation_id/profile。
func (h *Handler) UpdateGroupProfile(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	conversationID, ok := parseConversationID(c)
	if !ok {
		return
	}
	var request updateGroupProfileRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数格式错误"})
		return
	}
	result, err := h.service.UpdateGroupProfile(
		c.Request.Context(),
		UpdateGroupProfileCommand{
			ActorID:        userID,
			ConversationID: conversationID,
			Name:           request.Name,
			AvatarURL:      request.AvatarURL,
		},
	)
	if err != nil {
		writeConversationError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"id":         strconv.FormatUint(result.ConversationID, 10),
		"name":       result.Name,
		"avatar_url": result.AvatarURL,
	})
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
		"title":             result.Title,
		"avatar_url":        result.AvatarURL,
		"last_seq":          result.LastSeq,
		"last_read_seq":     result.LastReadSeq,
		"unread_count":      result.UnreadCount,
		"member_count":      result.MemberCount,
	}
}

func parseConversationID(c *gin.Context) (uint64, bool) {
	conversationID, err := strconv.ParseUint(
		c.Param("conversation_id"),
		10,
		64,
	)
	if err != nil || conversationID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "conversation_id 格式错误"})
		return 0, false
	}
	return conversationID, true
}

func parsePathUserID(c *gin.Context, parameter string) (uint64, bool) {
	userID, err := strconv.ParseUint(c.Param(parameter), 10, 64)
	if err != nil || userID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": parameter + " 格式错误"})
		return 0, false
	}
	return userID, true
}

func parseGroupMemberIDs(
	c *gin.Context,
	rawIDs []string,
) ([]uint64, bool) {
	result := make([]uint64, 0, len(rawIDs))
	for _, rawID := range rawIDs {
		userID, err := strconv.ParseUint(rawID, 10, 64)
		if err != nil || userID == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "member_user_ids 格式错误"})
			return nil, false
		}
		result = append(result, userID)
	}
	return result, true
}

func writeConversationError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrInvalidInput), errors.Is(err, ErrSameUser):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, ErrNotFriend):
		c.JSON(http.StatusForbidden, gin.H{"error": ErrNotFriend.Error()})
	case errors.Is(err, ErrPermissionDenied):
		c.JSON(http.StatusForbidden, gin.H{"error": ErrPermissionDenied.Error()})
	case errors.Is(err, ErrNotGroup):
		c.JSON(http.StatusBadRequest, gin.H{"error": ErrNotGroup.Error()})
	case errors.Is(err, ErrGroupFull):
		c.JSON(http.StatusConflict, gin.H{"error": ErrGroupFull.Error()})
	case errors.Is(err, ErrOwnerCannotLeave):
		c.JSON(http.StatusConflict, gin.H{"error": ErrOwnerCannotLeave.Error()})
	case errors.Is(err, ErrMemberNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": ErrMemberNotFound.Error()})
	case errors.Is(err, ErrUserNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": ErrUserNotFound.Error()})
	case errors.Is(err, ErrConversationNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": ErrConversationNotFound.Error()})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "服务器内部错误"})
	}
}
