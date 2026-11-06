package identity

import (
	"errors"
	"ginChatProject/internal/middleware"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// Handler 处理用户认证 HTTP 请求。
type Handler struct {
	service *Service
}

// NewHandler 创建用户认证 Handler。
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
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

	result, err := h.service.Login(c.Request.Context(), LoginCommand{
		Username: request.Username,
		Password: request.Password,
	})
	if err != nil {
		writeIdentityError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"token": result.Token,
		"user": gin.H{
			"id":       strconv.FormatUint(result.User.ID, 10),
			"username": result.User.Username,
			"nickname": result.User.Nickname,
		},
	})
}

// Me 处理 GET /api/v1/me。
func (h *Handler) Me(c *gin.Context) {
	userID, exists := c.Get(middleware.ContextUserIDKey)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未获取到登录用户"})
		return
	}

	userID64, ok := userID.(uint64)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "登录用户信息无效"})
		return
	}

	result, err := h.service.Me(c.Request.Context(), userID64)
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
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "服务器内部错误"})
	}
}
