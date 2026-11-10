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

const maxProfileBodyBytes = 16 << 10

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

type updateProfileRequest struct {
	// Nickname 是新的公开昵称。
	Nickname string `json:"nickname"`

	// AvatarURL 是可选头像地址，空字符串表示清除头像。
	AvatarURL *string `json:"avatar_url"`

	// Bio 是可选个人简介，空字符串表示清除简介。
	Bio string `json:"bio"`
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

	c.JSON(http.StatusCreated, userResponse(result))
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
		"user":               userResponse(result.User),
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

	c.JSON(http.StatusOK, userResponse(result))
}

// UpdateProfile 处理 PUT /api/v1/me。
func (h *Handler) UpdateProfile(c *gin.Context) {
	userID, ok := middleware.CurrentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "登录用户信息无效"})
		return
	}
	c.Request.Body = http.MaxBytesReader(
		c.Writer,
		c.Request.Body,
		maxProfileBodyBytes,
	)
	var request updateProfileRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "请求体过大"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数格式错误"})
		return
	}
	result, err := h.service.UpdateProfile(
		c.Request.Context(),
		UpdateProfileCommand{
			UserID:    userID,
			Nickname:  request.Nickname,
			AvatarURL: request.AvatarURL,
			Bio:       request.Bio,
		},
	)
	if err != nil {
		writeIdentityError(c, err)
		return
	}
	c.JSON(http.StatusOK, userResponse(result))
}

// UploadAvatar 处理 POST /api/v1/me/avatar。
func (h *Handler) UploadAvatar(c *gin.Context) {
	userID, ok := middleware.CurrentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "登录用户信息无效"})
		return
	}
	c.Request.Body = http.MaxBytesReader(
		c.Writer,
		c.Request.Body,
		maxAvatarBytes+(1<<20),
	)
	fileHeader, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请选择头像图片"})
		return
	}
	file, err := fileHeader.Open()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "读取头像图片失败"})
		return
	}
	defer file.Close()

	detectBuffer := make([]byte, 512)
	n, readErr := io.ReadFull(file, detectBuffer)
	if readErr != nil &&
		!errors.Is(readErr, io.ErrUnexpectedEOF) &&
		!errors.Is(readErr, io.EOF) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "读取头像图片失败"})
		return
	}
	contentType := http.DetectContentType(detectBuffer[:n])
	if _, supported := avatarExtensions[contentType]; !supported {
		c.JSON(
			http.StatusBadRequest,
			gin.H{"error": "头像仅支持 JPEG、PNG、WebP 或 GIF"},
		)
		return
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "处理头像图片失败"})
		return
	}

	result, err := h.service.UploadAvatar(
		c.Request.Context(),
		UploadAvatarCommand{
			UserID:      userID,
			ContentType: contentType,
			Size:        fileHeader.Size,
			Reader:      file,
		},
	)
	if err != nil {
		writeIdentityError(c, err)
		return
	}
	c.JSON(http.StatusOK, userResponse(result))
}

// RemoveAvatar 处理 DELETE /api/v1/me/avatar。
func (h *Handler) RemoveAvatar(c *gin.Context) {
	userID, ok := middleware.CurrentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "登录用户信息无效"})
		return
	}
	result, err := h.service.RemoveAvatar(c.Request.Context(), userID)
	if err != nil {
		writeIdentityError(c, err)
		return
	}
	c.JSON(http.StatusOK, userResponse(result))
}

// OpenAvatar 处理公开头像读取 GET /api/v1/users/:user_id/avatar。
func (h *Handler) OpenAvatar(c *gin.Context) {
	userID, err := strconv.ParseUint(c.Param("user_id"), 10, 64)
	if err != nil || userID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user_id 格式错误"})
		return
	}
	result, err := h.service.OpenAvatar(c.Request.Context(), userID)
	if err != nil {
		writeIdentityError(c, err)
		return
	}
	defer result.Reader.Close()
	c.Header("Cache-Control", "public, max-age=300")
	c.Header("X-Content-Type-Options", "nosniff")
	c.DataFromReader(
		http.StatusOK,
		result.SizeBytes,
		result.ContentType,
		result.Reader,
		nil,
	)
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

// writeIdentityError 把身份错误映射为 HTTP 响应
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
	case errors.Is(err, ErrInvalidAvatar):
		c.JSON(http.StatusBadRequest, gin.H{"error": ErrInvalidAvatar.Error()})
	case errors.Is(err, ErrAvatarNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": ErrAvatarNotFound.Error()})
	case errors.Is(err, ErrAvatarStorageUnavailable):
		c.JSON(
			http.StatusServiceUnavailable,
			gin.H{"error": ErrAvatarStorageUnavailable.Error()},
		)
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "服务器内部错误"})
	}
}

// userResponse 把用户模型转换为不含敏感字段的响应
func userResponse(result UserResult) gin.H {
	return gin.H{
		"id":         strconv.FormatUint(result.ID, 10),
		"username":   result.Username,
		"nickname":   result.Nickname,
		"avatar_url": result.AvatarURL,
		"bio":        result.Bio,
	}
}
