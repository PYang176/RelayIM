package friendship

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/PYang176/RelayIM/internal/middleware"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/PYang176/RelayIM/internal/platform/observability"
	"github.com/PYang176/RelayIM/internal/ws"
	"github.com/gin-gonic/gin"
)

// RelationshipDelivery 把好友关系变化通知到在线用户。
type RelationshipDelivery interface {
	Route(ctx context.Context, userIDs []uint64, payload []byte) error
}

// Handler 处理好友 HTTP 请求。
type Handler struct {
	service  *Service
	delivery RelationshipDelivery
}

// NewHandler 创建好友 Handler。
func NewHandler(service *Service, delivery RelationshipDelivery) *Handler {
	return &Handler{
		service:  service,
		delivery: delivery,
	}
}

type sendRequestPayload struct {
	// TargetUserID 是目标用户 Snowflake ID 字符串。
	TargetUserID string `json:"target_user_id"`

	// Remark 是可选申请附言。
	Remark string `json:"remark"`
}

type remarkPayload struct {
	// Remark 是好友备注，空字符串表示删除备注。
	Remark string `json:"remark"`
}

// SearchUsers 处理 GET /api/v1/users/search。
func (h *Handler) SearchUsers(c *gin.Context) {
	userID, ok := middleware.CurrentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "登录用户信息无效"})
		return
	}
	results, err := h.service.SearchUsers(
		c.Request.Context(),
		userID,
		c.Query("q"),
	)
	if err != nil {
		writeFriendshipError(c, err)
		return
	}
	response := make([]userResponse, 0, len(results))
	for _, result := range results {
		response = append(response, toUserResponse(result))
	}
	c.JSON(http.StatusOK, gin.H{"users": response})
}

// SendRequest 创建好友申请并处理重复申请、拉黑和唯一约束
func (h *Handler) SendRequest(c *gin.Context) {
	userID, ok := middleware.CurrentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "登录用户信息无效"})
		return
	}
	var payload sendRequestPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数格式错误"})
		return
	}
	targetUserID, err := strconv.ParseUint(payload.TargetUserID, 10, 64)
	if err != nil || targetUserID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "target_user_id 格式错误"})
		return
	}
	if err := h.service.SendRequest(
		c.Request.Context(),
		userID,
		targetUserID,
		payload.Remark,
	); err != nil {
		writeFriendshipError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"message": "好友申请已发送"})
}

// ListReceivedRequests 处理收到的好友申请。
func (h *Handler) ListReceivedRequests(c *gin.Context) {
	h.listRequests(c, true)
}

// ListSentRequests 处理发出的好友申请。
func (h *Handler) ListSentRequests(c *gin.Context) {
	h.listRequests(c, false)
}

// listRequests 查询好友申请列表并应用游标分页
func (h *Handler) listRequests(c *gin.Context, received bool) {
	userID, ok := middleware.CurrentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "登录用户信息无效"})
		return
	}
	page, ok := parsePageQuery(c)
	if !ok {
		return
	}
	result, err := h.service.ListRequests(
		c.Request.Context(),
		userID,
		received,
		page,
	)
	if err != nil {
		writeFriendshipError(c, err)
		return
	}
	response := make([]requestResponse, 0, len(result.Requests))
	for _, item := range result.Requests {
		response = append(response, requestResponse{
			ID:        strconv.FormatUint(item.ID, 10),
			User:      toUserResponse(item.User),
			Status:    item.Status,
			Remark:    item.Remark,
			CreatedAt: item.CreatedAt,
		})
	}
	c.JSON(http.StatusOK, gin.H{
		"requests":    response,
		"next_cursor": formatCursor(result.NextCursor),
		"has_more":    result.HasMore,
	})
}

// AcceptRequest 处理同意好友申请。
func (h *Handler) AcceptRequest(c *gin.Context) {
	h.respondRequest(c, true)
}

// RejectRequest 处理拒绝好友申请。
func (h *Handler) RejectRequest(c *gin.Context) {
	h.respondRequest(c, false)
}

// respondRequest 处理好友申请审批并推进申请状态
func (h *Handler) respondRequest(c *gin.Context, accept bool) {
	userID, ok := middleware.CurrentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "登录用户信息无效"})
		return
	}
	requestID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || requestID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "申请 ID 格式错误"})
		return
	}

	if accept {
		err = h.service.AcceptRequest(c.Request.Context(), userID, requestID)
	} else {
		err = h.service.RejectRequest(c.Request.Context(), userID, requestID)
	}
	if err != nil {
		writeFriendshipError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "好友申请已处理"})
}

// ListFriends 查询当前用户的好友关系并应用游标分页
func (h *Handler) ListFriends(c *gin.Context) {
	userID, ok := middleware.CurrentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "登录用户信息无效"})
		return
	}
	page, ok := parsePageQuery(c)
	if !ok {
		return
	}
	result, err := h.service.ListFriends(c.Request.Context(), userID, page)
	if err != nil {
		writeFriendshipError(c, err)
		return
	}
	response := make([]friendResponse, 0, len(result.Friends))
	for _, item := range result.Friends {
		response = append(response, friendResponse{
			FriendshipID:       strconv.FormatUint(item.FriendshipID, 10),
			User:               toUserResponse(item.User),
			Remark:             item.Remark,
			BlockedByMe:        item.BlockedByMe,
			RelationshipActive: item.RelationshipActive,
		})
	}
	c.JSON(http.StatusOK, gin.H{
		"friends":     response,
		"next_cursor": formatCursor(result.NextCursor),
		"has_more":    result.HasMore,
	})
}

// RelationshipStatus 处理 GET /api/v1/friends/:user_id/relationship。
func (h *Handler) RelationshipStatus(c *gin.Context) {
	userID, ok := middleware.CurrentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "登录用户信息无效"})
		return
	}
	targetUserID, err := strconv.ParseUint(c.Param("user_id"), 10, 64)
	if err != nil || targetUserID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "用户 ID 格式错误"})
		return
	}
	result, err := h.service.RelationshipStatus(
		c.Request.Context(),
		userID,
		targetUserID,
	)
	if err != nil {
		writeFriendshipError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"target_user_id":      strconv.FormatUint(result.TargetUserID, 10),
		"can_message":         result.CanMessage,
		"can_request":         result.CanRequest,
		"has_pending_request": result.HasPendingRequest,
		"requested_by_me":     result.RequestedByMe,
		"blocked_by_me":       result.BlockedByMe,
	})
}

// ListBlockedUsers 处理当前用户的拉黑列表。
func (h *Handler) ListBlockedUsers(c *gin.Context) {
	userID, ok := middleware.CurrentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "登录用户信息无效"})
		return
	}
	page, ok := parsePageQuery(c)
	if !ok {
		return
	}
	result, err := h.service.ListBlockedUsers(
		c.Request.Context(),
		userID,
		page,
	)
	if err != nil {
		writeFriendshipError(c, err)
		return
	}
	response := make([]blockedUserResponse, 0, len(result.BlockedUsers))
	for _, item := range result.BlockedUsers {
		response = append(response, blockedUserResponse{
			User:      toUserResponse(item.User),
			CreatedAt: item.CreatedAt,
		})
	}
	c.JSON(http.StatusOK, gin.H{
		"blocked_users": response,
		"next_cursor":   formatCursor(result.NextCursor),
		"has_more":      result.HasMore,
	})
}

// BlockUser 处理拉黑用户。
func (h *Handler) BlockUser(c *gin.Context) {
	h.changeBlock(c, true)
}

// UnblockUser 处理取消拉黑。
func (h *Handler) UnblockUser(c *gin.Context) {
	h.changeBlock(c, false)
}

// changeBlock 执行拉黑状态变更
func (h *Handler) changeBlock(c *gin.Context, block bool) {
	userID, ok := middleware.CurrentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "登录用户信息无效"})
		return
	}
	targetUserID, err := strconv.ParseUint(c.Param("user_id"), 10, 64)
	if err != nil || targetUserID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "用户 ID 格式错误"})
		return
	}

	if block {
		err = h.service.BlockUser(c.Request.Context(), userID, targetUserID)
	} else {
		err = h.service.UnblockUser(c.Request.Context(), userID, targetUserID)
	}
	if err != nil {
		writeFriendshipError(c, err)
		return
	}
	message := "已取消拉黑"
	if block {
		message = "已拉黑用户"
	}
	c.JSON(http.StatusOK, gin.H{"message": message})
}

// SetRemark 处理好友备注更新。
func (h *Handler) SetRemark(c *gin.Context) {
	userID, ok := middleware.CurrentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "登录用户信息无效"})
		return
	}
	friendUserID, err := strconv.ParseUint(c.Param("user_id"), 10, 64)
	if err != nil || friendUserID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "好友用户 ID 格式错误"})
		return
	}
	var payload remarkPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数格式错误"})
		return
	}
	if err := h.service.SetRemark(
		c.Request.Context(),
		userID,
		friendUserID,
		payload.Remark,
	); err != nil {
		writeFriendshipError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "好友备注已更新"})
}

// DeleteFriend 处理删除好友。
func (h *Handler) DeleteFriend(c *gin.Context) {
	userID, ok := middleware.CurrentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "登录用户信息无效"})
		return
	}
	friendUserID, err := strconv.ParseUint(c.Param("user_id"), 10, 64)
	if err != nil || friendUserID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "好友用户 ID 格式错误"})
		return
	}
	if err := h.service.DeleteFriend(
		c.Request.Context(),
		userID,
		friendUserID,
	); err != nil {
		writeFriendshipError(c, err)
		return
	}
	h.notifyRelationshipChanged(
		c.Request.Context(),
		userID,
		friendUserID,
		false,
		false,
	)
	h.notifyRelationshipChanged(
		c.Request.Context(),
		friendUserID,
		userID,
		false,
		true,
	)
	c.JSON(http.StatusOK, gin.H{"message": "好友已删除"})
}

// notifyRelationshipChanged 通知单个用户刷新联系人及私聊发送权限。
func (h *Handler) notifyRelationshipChanged(
	ctx context.Context,
	targetUserID uint64,
	peerUserID uint64,
	active bool,
	visible bool,
) {
	if h.delivery == nil || targetUserID == 0 || peerUserID == 0 {
		return
	}
	payload, err := json.Marshal(ws.FriendRelationshipMessage{
		Action:             ws.ActionFriendRelationshipChanged,
		PeerUserID:         strconv.FormatUint(peerUserID, 10),
		RelationshipActive: active,
		FriendVisible:      visible,
		TraceID:            observability.TraceID(ctx),
	})
	if err != nil {
		slog.Warn("序列化好友关系通知失败", "error", err)
		return
	}
	if err := h.delivery.Route(ctx, []uint64{targetUserID}, payload); err != nil {
		slog.Warn(
			"推送好友关系通知失败，等待客户端下次同步",
			"target_user_id",
			targetUserID,
			"peer_user_id",
			peerUserID,
			"error",
			err,
		)
	}
}

type userResponse struct {
	// ID 是用户 Snowflake ID 字符串。
	ID string `json:"id"`

	// Username 是登录名。
	Username string `json:"username"`

	// Nickname 是公开昵称。
	Nickname string `json:"nickname"`

	// AvatarURL 是头像地址。
	AvatarURL string `json:"avatar_url"`
}

type friendResponse struct {
	// FriendshipID 是好友关系 Snowflake ID 字符串。
	FriendshipID string `json:"friendship_id"`

	// User 是好友公开资料。
	User userResponse `json:"user"`

	// Remark 是当前用户设置的独立备注。
	Remark string `json:"remark"`

	// BlockedByMe 表示当前用户是否已拉黑该好友。
	BlockedByMe bool `json:"blocked_by_me"`

	// RelationshipActive 表示双方当前是否仍维持有效好友关系。
	RelationshipActive bool `json:"relationship_active"`
}

type blockedUserResponse struct {
	// User 是被拉黑用户公开资料。
	User userResponse `json:"user"`

	// CreatedAt 是拉黑关系创建的 UTC 时间。
	CreatedAt time.Time `json:"created_at"`
}

type requestResponse struct {
	// ID 是好友申请 Snowflake ID 字符串。
	ID string `json:"id"`

	// User 是申请对方公开资料。
	User userResponse `json:"user"`

	// Status 表示申请状态。
	Status int `json:"status"`

	// Remark 是申请附言。
	Remark string `json:"remark"`

	// CreatedAt 是申请创建时间。
	CreatedAt time.Time `json:"created_at"`
}

// toUserResponse 把用户及其资料转换为好友列表响应
func toUserResponse(result UserResult) userResponse {
	return userResponse{
		ID:        strconv.FormatUint(result.ID, 10),
		Username:  result.Username,
		Nickname:  result.Nickname,
		AvatarURL: result.AvatarURL,
	}
}

// parsePageQuery 解析分页查询并校验参数边界
func parsePageQuery(c *gin.Context) (PageQuery, bool) {
	page := PageQuery{Limit: defaultPageSize}
	if rawLimit := c.Query("limit"); rawLimit != "" {
		limit, err := strconv.Atoi(rawLimit)
		if err != nil || limit <= 0 || limit > maxPageSize {
			c.JSON(http.StatusBadRequest, gin.H{"error": "limit 必须在 1 到 100 之间"})
			return PageQuery{}, false
		}
		page.Limit = limit
	}
	if rawCursor := c.Query("before_id"); rawCursor != "" {
		cursor, err := strconv.ParseUint(rawCursor, 10, 64)
		if err != nil || cursor == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "before_id 格式错误"})
			return PageQuery{}, false
		}
		page.BeforeID = cursor
	}
	return page, true
}

// formatCursor 把业务游标格式化为客户端可回传的字符串
func formatCursor(cursor uint64) string {
	if cursor == 0 {
		return ""
	}
	return strconv.FormatUint(cursor, 10)
}

// writeFriendshipError 把好友业务错误映射为 HTTP 状态和响应
func writeFriendshipError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrInvalidInput):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, ErrRequestRemarkRequired):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, ErrUserNotFound), errors.Is(err, ErrRequestNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	case errors.Is(err, ErrRequestForbidden):
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
	case errors.Is(err, ErrUserBlocked):
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
	case errors.Is(err, ErrAlreadyFriends), errors.Is(err, ErrRequestPending):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	case errors.Is(err, ErrFriendshipNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "服务器内部错误"})
	}
}
