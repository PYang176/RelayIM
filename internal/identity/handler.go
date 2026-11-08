package identity

import (
	"context"
	"errors"
	"ginChatProject/internal/middleware"
	"ginChatProject/internal/platform/authn"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

// Handler 处理用户认证 HTTP 请求。
type Handler struct {
	service   *Service
	wsTickets WSTicketIssuer
}

// WSTicketIssuer 签发短时、单次使用的 WebSocket 建连票据。
type WSTicketIssuer interface {
	IssueWSTicket(
		ctx context.Context,
		userID uint64,
		deviceID uint64,
	) (string, time.Duration, error)
}

// NewHandler 创建用户认证 Handler。
func NewHandler(service *Service, wsTickets WSTicketIssuer) *Handler {
	return &Handler{
		service:   service,
		wsTickets: wsTickets,
	}
}

type registerRequest struct {
	// Username 是唯一登录名。
	Username string `json:"username"`

	// Password 是登录密码。
	Password string `json:"password"`

	// ConfirmPassword 是密码确认。
	ConfirmPassword string `json:"confirm_password"`

	// Nickname 是可选公开昵称。
	Nickname string `json:"nickname"`
}

type loginRequest struct {
	// Username 是登录名。
	Username string `json:"username"`

	// Password 是登录密码。
	Password string `json:"password"`

	// DeviceID 是客户端已保存的设备 Snowflake ID 字符串。
	DeviceID string `json:"device_id"`

	// DeviceName 是登录设备名称。
	DeviceName string `json:"device_name"`

	// Platform 表示设备平台：1 Web、2 Android、3 iOS、4 Desktop。
	Platform int `json:"platform"`
}

type refreshRequest struct {
	// RefreshToken 是客户端持有的原始 Refresh Token。
	RefreshToken string `json:"refresh_token"`
}

type logoutRequest struct {
	// RefreshToken 是可选原始 Refresh Token。
	RefreshToken string `json:"refresh_token"`
}

type wsTicketRequest struct {
	// DeviceID 是客户端已保存的设备 Snowflake ID 字符串。
	DeviceID string `json:"device_id"`

	// DeviceName 是设备展示名称。
	DeviceName string `json:"device_name"`

	// Platform 表示设备平台：1 Web、2 Android、3 iOS、4 Desktop。
	Platform int `json:"platform"`
}

// Register 处理 POST /api/v1/auth/register。
func (h *Handler) Register(c *gin.Context) {
	var request registerRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数格式错误"})
		return
	}

	result, err := h.service.Register(c.Request.Context(), RegisterCommand{
		Username:        request.Username,
		Password:        request.Password,
		ConfirmPassword: request.ConfirmPassword,
		Nickname:        request.Nickname,
	})
	if err != nil {
		writeIdentityError(c, err)
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"id":       strconv.FormatUint(result.ID, 10),
		"username": result.Username,
		"nickname": result.Nickname,
	})
}

// Login 处理 POST /api/v1/auth/login。
func (h *Handler) Login(c *gin.Context) {
	var request loginRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数格式错误"})
		return
	}

	var deviceID uint64
	if request.DeviceID != "" {
		parsed, err := strconv.ParseUint(request.DeviceID, 10, 64)
		if err != nil || parsed == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "device_id 格式错误"})
			return
		}
		deviceID = parsed
	}
	result, err := h.service.Login(c.Request.Context(), LoginCommand{
		Username:   request.Username,
		Password:   request.Password,
		DeviceID:   deviceID,
		DeviceName: request.DeviceName,
		Platform:   request.Platform,
	})
	if err != nil {
		writeIdentityError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"token":              result.Token,
		"refresh_token":      result.RefreshToken,
		"refresh_expires_at": result.RefreshExpiresAt,
		"device_id":          strconv.FormatUint(result.DeviceID, 10),
		"user": gin.H{
			"id":       strconv.FormatUint(result.User.ID, 10),
			"username": result.User.Username,
			"nickname": result.User.Nickname,
		},
	})
}

// Refresh 处理 POST /api/v1/auth/refresh。
func (h *Handler) Refresh(c *gin.Context) {
	var request refreshRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数格式错误"})
		return
	}
	result, err := h.service.Refresh(
		c.Request.Context(),
		RefreshCommand{RefreshToken: request.RefreshToken},
	)
	if err != nil {
		writeIdentityError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"token":              result.Token,
		"refresh_token":      result.RefreshToken,
		"refresh_expires_at": result.RefreshExpiresAt,
	})
}

// Logout 处理 POST /api/v1/auth/logout。
func (h *Handler) Logout(c *gin.Context) {
	userID, ok := middleware.CurrentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "登录用户信息无效"})
		return
	}
	token, _ := c.Get(middleware.ContextTokenKey)
	jti, _ := c.Get(middleware.ContextJTIKey)
	expiresAt, _ := c.Get(middleware.ContextExpiresAtKey)
	accessToken, _ := token.(string)
	accessJTI, _ := jti.(string)
	accessExpiresAt, _ := expiresAt.(time.Time)

	var request logoutRequest
	if err := c.ShouldBindJSON(&request); err != nil &&
		!errors.Is(err, io.EOF) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数格式错误"})
		return
	}
	if err := h.service.Logout(
		c.Request.Context(),
		LogoutCommand{
			UserID:          userID,
			AccessToken:     accessToken,
			JTI:             accessJTI,
			AccessExpiresAt: accessExpiresAt,
			RefreshToken:    request.RefreshToken,
		},
	); err != nil {
		writeIdentityError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// Me 处理 GET /api/v1/me。
func (h *Handler) Me(c *gin.Context) {
	userID, ok := middleware.CurrentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "登录用户信息无效"})
		return
	}

	result, err := h.service.Me(c.Request.Context(), userID)
	if err != nil {
		writeIdentityError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"id":       strconv.FormatUint(result.ID, 10),
		"username": result.Username,
		"nickname": result.Nickname,
	})
}

// WSTicket 处理 POST /api/v1/auth/ws-ticket。
func (h *Handler) WSTicket(c *gin.Context) {
	userID, ok := middleware.CurrentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "登录用户信息无效"})
		return
	}

	var request wsTicketRequest
	if err := c.ShouldBindJSON(&request); err != nil && !errors.Is(err, io.EOF) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数格式错误"})
		return
	}

	var deviceID uint64
	if request.DeviceID != "" {
		parsed, err := strconv.ParseUint(request.DeviceID, 10, 64)
		if err != nil || parsed == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "device_id 格式错误"})
			return
		}
		deviceID = parsed
	}

	device, err := h.service.EnsureDevice(c.Request.Context(), EnsureDeviceCommand{
		UserID:     userID,
		DeviceID:   deviceID,
		DeviceName: request.DeviceName,
		Platform:   request.Platform,
	})
	if err != nil {
		writeIdentityError(c, err)
		return
	}

	ticket, ttl, err := h.wsTickets.IssueWSTicket(
		c.Request.Context(),
		userID,
		device.ID,
	)
	if err != nil {
		if errors.Is(err, authn.ErrAuthUnavailable) {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "认证服务暂不可用"})
			return
		}
		c.JSON(http.StatusUnauthorized, gin.H{"error": "登录用户信息无效"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"ticket":             ticket,
		"expires_in_seconds": int64(ttl / time.Second),
		"device_id":          strconv.FormatUint(device.ID, 10),
		"device_name":        device.DeviceName,
		"platform":           device.Platform,
	})
}

func writeIdentityError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrInvalidInput):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, ErrInvalidCredentials):
		c.JSON(http.StatusUnauthorized, gin.H{"error": ErrInvalidCredentials.Error()})
	case errors.Is(err, ErrAccountDisabled):
		c.JSON(http.StatusForbidden, gin.H{"error": ErrAccountDisabled.Error()})
	case errors.Is(err, ErrUsernameExists):
		c.JSON(http.StatusConflict, gin.H{"error": ErrUsernameExists.Error()})
	case errors.Is(err, ErrUserNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": ErrUserNotFound.Error()})
	case errors.Is(err, ErrDeviceNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": ErrDeviceNotFound.Error()})
	case errors.Is(err, ErrInvalidRefreshToken):
		c.JSON(http.StatusUnauthorized, gin.H{"error": ErrInvalidRefreshToken.Error()})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "服务器内部错误"})
	}
}
