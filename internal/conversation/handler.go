package conversation

import (
	"encoding/base64"
	"errors"
	"ginChatProject/internal/middleware"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

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

	// InviteRemark 是邀请人填写的入群理由。
	InviteRemark string `json:"invite_remark"`
}

type inviteMembersRequest struct {
	// MemberUserIDs 是继续邀请入群的好友 Snowflake ID 字符串。
	MemberUserIDs []string `json:"member_user_ids"`

	// InviteRemark 是邀请人填写的入群理由。
	InviteRemark string `json:"invite_remark"`
}

type respondInvitationRequest struct {
	// ResponseRemark 是可选审批回复。
	ResponseRemark string `json:"response_remark"`
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
			InviteRemark:  request.InviteRemark,
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
	page, ok := parseConversationPageQuery(c)
	if !ok {
		return
	}

	result, err := h.service.List(c.Request.Context(), userID, page)
	if err != nil {
		writeConversationError(c, err)
		return
	}

	response := make([]gin.H, 0, len(result.Conversations))
	for _, conversation := range result.Conversations {
		response = append(response, conversationResponse(conversation))
	}
	c.JSON(http.StatusOK, gin.H{
		"conversations": response,
		"next_cursor":   encodeConversationCursor(result.NextUpdatedAt, result.NextID),
		"has_more":      result.HasMore,
	})
}

// Get 处理 GET /api/v1/conversations/:conversation_id。
func (h *Handler) Get(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	conversationID, ok := parseConversationID(c)
	if !ok {
		return
	}
	result, err := h.service.Get(c.Request.Context(), userID, conversationID)
	if err != nil {
		writeConversationError(c, err)
		return
	}
	c.JSON(http.StatusOK, conversationResponse(result))
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
	invitedCount, err := h.service.InviteMembers(
		c.Request.Context(),
		InviteMembersCommand{
			ActorID:        userID,
			ConversationID: conversationID,
			MemberUserIDs:  memberUserIDs,
			InviteRemark:   request.InviteRemark,
		},
	)
	if err != nil {
		writeConversationError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"invited_count": invitedCount})
}

// ListReceivedInvitations 处理 GET /api/v1/group-invitations。
func (h *Handler) ListReceivedInvitations(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	page, ok := parseInvitationPageQuery(c)
	if !ok {
		return
	}
	result, err := h.service.ListReceivedInvitations(
		c.Request.Context(),
		userID,
		page,
	)
	if err != nil {
		writeConversationError(c, err)
		return
	}
	invitations := make([]gin.H, 0, len(result.Invitations))
	for _, invitation := range result.Invitations {
		invitations = append(invitations, gin.H{
			"invitation_id":     strconv.FormatUint(invitation.InvitationID, 10),
			"conversation_id":   strconv.FormatUint(invitation.ConversationID, 10),
			"conversation_name": invitation.ConversationName,
			"avatar_url":        invitation.AvatarURL,
			"inviter_id":        strconv.FormatUint(invitation.InviterID, 10),
			"inviter_name":      invitation.InviterName,
			"invite_remark":     invitation.InviteRemark,
			"created_at":        invitation.CreatedAt,
			"expires_at":        invitation.ExpiresAt,
		})
	}
	c.JSON(http.StatusOK, gin.H{
		"invitations": invitations,
		"next_cursor": formatInvitationCursor(result.NextCursor),
		"has_more":    result.HasMore,
	})
}

// AcceptInvitation 处理 POST /api/v1/group-invitations/:id/accept。
func (h *Handler) AcceptInvitation(c *gin.Context) {
	h.respondInvitation(c, true)
}

// RejectInvitation 处理 POST /api/v1/group-invitations/:id/reject。
func (h *Handler) RejectInvitation(c *gin.Context) {
	h.respondInvitation(c, false)
}

func (h *Handler) respondInvitation(c *gin.Context, accept bool) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	invitationID, ok := parsePathUserID(c, "id")
	if !ok {
		return
	}
	responseRemark, ok := parseInvitationResponse(c)
	if !ok {
		return
	}
	command := RespondInvitationCommand{
		UserID:         userID,
		InvitationID:   invitationID,
		ResponseRemark: responseRemark,
	}
	if !accept {
		if err := h.service.RejectInvitation(
			c.Request.Context(),
			command,
		); err != nil {
			writeConversationError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "群邀请已拒绝"})
		return
	}
	result, err := h.service.AcceptInvitation(
		c.Request.Context(),
		command,
	)
	if err != nil {
		writeConversationError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"invitation_id":   strconv.FormatUint(result.InvitationID, 10),
		"conversation_id": strconv.FormatUint(result.ConversationID, 10),
		"member_count":    result.MemberCount,
	})
}

// ListGroupInvitations 处理群待审批邀请查询。
func (h *Handler) ListGroupInvitations(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	conversationID, ok := parseConversationID(c)
	if !ok {
		return
	}
	page, ok := parseInvitationPageQuery(c)
	if !ok {
		return
	}
	result, err := h.service.ListGroupInvitations(
		c.Request.Context(),
		userID,
		conversationID,
		page,
	)
	if err != nil {
		writeConversationError(c, err)
		return
	}
	invitations := make([]gin.H, 0, len(result.Invitations))
	for _, invitation := range result.Invitations {
		invitations = append(invitations, gin.H{
			"invitation_id": strconv.FormatUint(invitation.InvitationID, 10),
			"invitee": gin.H{
				"user_id":    strconv.FormatUint(invitation.Invitee.UserID, 10),
				"username":   invitation.Invitee.Username,
				"nickname":   invitation.Invitee.Nickname,
				"avatar_url": invitation.Invitee.AvatarURL,
			},
			"inviter_id":      strconv.FormatUint(invitation.InviterID, 10),
			"invite_remark":   invitation.InviteRemark,
			"status":          invitation.Status,
			"response_remark": invitation.ResponseRemark,
			"created_at":      invitation.CreatedAt,
			"expires_at":      invitation.ExpiresAt,
			"responded_at":    invitation.RespondedAt,
		})
	}
	c.JSON(http.StatusOK, gin.H{
		"invitations": invitations,
		"next_cursor": formatInvitationCursor(result.NextCursor),
		"has_more":    result.HasMore,
	})
}

// CancelInvitation 处理群主或管理员取消待处理邀请。
func (h *Handler) CancelInvitation(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	conversationID, ok := parseConversationID(c)
	if !ok {
		return
	}
	invitationID, ok := parsePathUserID(c, "invitation_id")
	if !ok {
		return
	}
	if err := h.service.CancelInvitation(
		c.Request.Context(),
		CancelInvitationCommand{
			ActorID:        userID,
			ConversationID: conversationID,
			InvitationID:   invitationID,
		},
	); err != nil {
		writeConversationError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
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
	peerUserID := ""
	if result.PeerUserID > 0 {
		peerUserID = strconv.FormatUint(result.PeerUserID, 10)
	}
	return gin.H{
		"id":                strconv.FormatUint(result.ID, 10),
		"conversation_type": result.ConversationType,
		"name":              result.Name,
		"title":             result.Title,
		"peer_user_id":      peerUserID,
		"avatar_url":        result.AvatarURL,
		"last_seq":          result.LastSeq,
		"last_read_seq":     result.LastReadSeq,
		"unread_count":      result.UnreadCount,
		"member_count":      result.MemberCount,
		"updated_at":        result.UpdatedAt,
	}
}

func parseConversationPageQuery(
	c *gin.Context,
) (ConversationPageQuery, bool) {
	page := ConversationPageQuery{Limit: defaultPageSize}
	if rawLimit := c.Query("limit"); rawLimit != "" {
		limit, err := strconv.Atoi(rawLimit)
		if err != nil || limit <= 0 || limit > maxPageSize {
			c.JSON(
				http.StatusBadRequest,
				gin.H{"error": "limit 必须在 1 到 100 之间"},
			)
			return ConversationPageQuery{}, false
		}
		page.Limit = limit
	}
	if cursor := c.Query("cursor"); cursor != "" {
		beforeUpdatedAt, beforeID, err := decodeConversationCursor(cursor)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "cursor 格式错误"})
			return ConversationPageQuery{}, false
		}
		page.BeforeUpdatedAt = beforeUpdatedAt
		page.BeforeID = beforeID
	}
	return page, true
}

func encodeConversationCursor(updatedAt time.Time, conversationID uint64) string {
	if updatedAt.IsZero() || conversationID == 0 {
		return ""
	}
	raw := strconv.FormatInt(updatedAt.UnixMilli(), 10) +
		"|" +
		strconv.FormatUint(conversationID, 10)
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func decodeConversationCursor(value string) (time.Time, uint64, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return time.Time{}, 0, err
	}
	parts := strings.Split(string(decoded), "|")
	if len(parts) != 2 {
		return time.Time{}, 0, errors.New("游标字段数量错误")
	}
	milliseconds, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || milliseconds <= 0 {
		return time.Time{}, 0, errors.New("游标时间错误")
	}
	conversationID, err := strconv.ParseUint(parts[1], 10, 64)
	if err != nil || conversationID == 0 {
		return time.Time{}, 0, errors.New("游标会话 ID 错误")
	}
	return time.UnixMilli(milliseconds).UTC(), conversationID, nil
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

func parseInvitationPageQuery(
	c *gin.Context,
) (InvitationPageQuery, bool) {
	page := InvitationPageQuery{Limit: defaultPageSize}
	if rawLimit := c.Query("limit"); rawLimit != "" {
		limit, err := strconv.Atoi(rawLimit)
		if err != nil || limit <= 0 || limit > maxPageSize {
			c.JSON(
				http.StatusBadRequest,
				gin.H{"error": "limit 必须在 1 到 100 之间"},
			)
			return InvitationPageQuery{}, false
		}
		page.Limit = limit
	}
	if rawCursor := c.Query("before_id"); rawCursor != "" {
		cursor, err := strconv.ParseUint(rawCursor, 10, 64)
		if err != nil || cursor == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "before_id 格式错误"})
			return InvitationPageQuery{}, false
		}
		page.BeforeID = cursor
	}
	return page, true
}

func parseInvitationResponse(c *gin.Context) (string, bool) {
	if c.Request.ContentLength == 0 {
		return "", true
	}
	var request respondInvitationRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		if errors.Is(err, io.EOF) {
			return "", true
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数格式错误"})
		return "", false
	}
	return request.ResponseRemark, true
}

func formatInvitationCursor(cursor uint64) string {
	if cursor == 0 {
		return ""
	}
	return strconv.FormatUint(cursor, 10)
}

func writeConversationError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrInvalidInput), errors.Is(err, ErrSameUser):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, ErrNotFriend):
		c.JSON(http.StatusForbidden, gin.H{"error": ErrNotFriend.Error()})
	case errors.Is(err, ErrUserBlocked):
		c.JSON(http.StatusForbidden, gin.H{"error": ErrUserBlocked.Error()})
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
	case errors.Is(err, ErrInvitationRemarkRequired):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, ErrInvitationNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": ErrInvitationNotFound.Error()})
	case errors.Is(err, ErrInvitationForbidden):
		c.JSON(http.StatusForbidden, gin.H{"error": ErrInvitationForbidden.Error()})
	case errors.Is(err, ErrInvitationManageForbidden):
		c.JSON(http.StatusForbidden, gin.H{"error": ErrInvitationManageForbidden.Error()})
	case errors.Is(err, ErrInvitationHandled),
		errors.Is(err, ErrInvitationExpired),
		errors.Is(err, ErrInvitationUnavailable):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "服务器内部错误"})
	}
}
