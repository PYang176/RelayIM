package api

import (
	"context"
	"fmt"
	"ginChatProject/internal/dto"
	"ginChatProject/internal/middleware"
	"ginChatProject/internal/service"
	"github.com/gin-gonic/gin"
	"net"
	"net/http"
	"strconv"
	"time"
)

type UserHandler struct {
	svc     *service.UserService
	revoker TokenRevoker
}

// TokenRevoker 撤销当前登录凭证，Handler 不直接依赖 Redis。
type TokenRevoker interface {
	Revoke(ctx context.Context, token string, jti string, expiresAt time.Time) error
}

// NewUserHandler 构造函数依赖注入
func NewUserHandler(
	svc *service.UserService,
	revoker TokenRevoker,
) *UserHandler {
	return &UserHandler{
		svc:     svc,
		revoker: revoker,
	}
}

// List HTTP 接口：GET /users
// @Tags users
// @Produce json
// @Success 200 {object} map[string]string
// @Router /users [get]
func (h *UserHandler) List(c *gin.Context) {
	users, err := h.svc.GetUserList(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, users)
}

// Create
// @Summary 创建用户
// @Description 新增一个用户
// @Tags users
// @Accept json
// @Produce json
// @Param user body dto.UserCreateDTO true "用户信息"
// @Success 200 {object} map[string]string
// @Router /users [post]
func (h *UserHandler) Create(c *gin.Context) {
	var req dto.UserCreateDTO

	//绑定并校验JSON
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	//调用Service，和http请求生命周期绑定
	if err := h.svc.CreateUser(c.Request.Context(), &req); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "新增用户成功"})
}

// Delete
// @Summary 删除用户
// @Description 根据用户ID删除用户（逻辑删除）
// @Tags users
// @Param id path int true "用户ID"
// @Success 200 {object} map[string]string
// @Router /users/{id} [delete]
func (h *UserHandler) Delete(c *gin.Context) {
	//从 URL 中获取路径参数
	idStr := c.Param("id")

	// 字符串转 uint
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的用户ID"})
		return
	}

	//权限校验是否有权删除(只能删除自己)
	//从token中取userID
	operatorId := c.MustGet("user_id").(uint)
	fmt.Println("delete----------------", operatorId)
	fmt.Println("delete----------------", id)
	if operatorId != uint(id) {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权删除该用户"})
		return
	}

	del := dto.UserDeleteDTO{
		Id: uint(id),
	}

	// 调用 Service，和 HTTP 请求生命周期绑定
	if err := h.svc.DeleteUser(c.Request.Context(), &del); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "删除用户成功"})
}

// Update
// @Summary 更新用户
// @Tags users
// @Param id path int true "用户ID"
// @Param user body dto.UserUpdateDTO true "用户信息"
// @Success 200 {object} map[string]string
// @Router /users/{id} [put]
func (h *UserHandler) Update(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "非法用户ID"})
		return
	}

	//校验参数是否合法
	var req dto.UserUpdateDTO
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 从token中获取userId,进行权限比对
	operatorId := c.MustGet("user_id").(uint)
	req.ID = uint(id)
	if operatorId != req.ID {
		c.JSON(http.StatusForbidden, "无权修改信息")
		return
	}

	if err := h.svc.UpdateUser(c.Request.Context(), req); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "更新成功"})
}

// Login
// @Summary 用户登录
// @Description 用户登录并返回 JWT Token
// @Tags users
// @Accept json
// @Produce json
// @Param user body dto.UserLoginDTO true "登录信息"
// @Success 200 {object} map[string]string "token"
// @Failure 400 {object} map[string]string "参数错误"
// @Failure 401 {object} map[string]string "认证失败"
// @Router /users/login [post]
func (h *UserHandler) Login(c *gin.Context) {
	var req dto.UserLoginDTO

	//绑定并校验JSON
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 正确获取 IP 和端口
	clientIP, clientPort := getClientIPAndPort(c)

	now := time.Now()
	// 获取客户端信息
	deviceInfo := service.UserDeviceInfo{
		ClientIp:   clientIP,
		ClientPort: clientPort,
		LoginTime:  &now,
		DeviceInfo: c.GetHeader("User-Agent"),
	}

	token, err := h.svc.Login(c.Request.Context(), req, deviceInfo)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "登录成功",
		"token":   token,
	})
}

// Logout 退出登录
func (h *UserHandler) Logout(c *gin.Context) {
	// Token 已经由 AuthMiddleware 校验，这里直接使用上下文中的原始值和 JTI，
	// 避免再次解析带 "Bearer " 前缀的 Header。
	tokenStr, tokenExists := c.Get(middleware.ContextTokenKey)
	jti, jtiExists := c.Get(middleware.ContextJTIKey)
	expiresAt, expiresExists := c.Get(middleware.ContextExpiresAtKey)
	userID, userExists := c.Get(middleware.ContextLegacyUserIDKey)
	if !tokenExists || !jtiExists || !expiresExists || !userExists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "登录状态无效"})
		return
	}
	legacyUserID, userOK := userID.(uint)
	jtiValue, jtiOK := jti.(string)
	expiresAtValue, expiresOK := expiresAt.(time.Time)
	tokenValue, tokenOK := tokenStr.(string)
	if !userOK || !jtiOK || !expiresOK || !tokenOK {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "登录状态无效"})
		return
	}

	// 更新数据库：登出时间和状态
	now := time.Now()
	if err := h.svc.Logout(c.Request.Context(), legacyUserID, now); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "退出登录失败"})
		return
	}

	// 统一认证组件负责黑名单和白名单撤销，Handler 不直接操作 Redis。
	if err := h.revoker.Revoke(
		c.Request.Context(),
		tokenValue,
		jtiValue,
		expiresAtValue,
	); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "退出登录失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "退出成功"})
}

// 解析客户端 IP 和端口
func getClientIPAndPort(c *gin.Context) (string, string) {
	// 获取真实 IP（支持代理）
	clientIP := c.ClientIP()

	// 从 RemoteAddr 解析端口
	remoteAddr := c.Request.RemoteAddr
	host, port, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		// 如果没有端口，返回空
		return clientIP, ""
	}

	// 如果 clientIP 是代理的，使用 RemoteAddr 的 IP
	if clientIP == "127.0.0.1" || clientIP == "::1" {
		clientIP = host
	}

	return clientIP, port
}
