package api

import (
	"fmt"
	"ginChatProject/internal/dto"
	"ginChatProject/internal/platform/jwt"
	"ginChatProject/internal/platform/redisstore"
	"ginChatProject/internal/service"
	"github.com/gin-gonic/gin"
	"net"
	"net/http"
	"strconv"
	"time"
)

type UserHandler struct {
	svc *service.UserService
}

// NewUserHandler 构造函数依赖注入
func NewUserHandler(svc *service.UserService) *UserHandler {
	return &UserHandler{svc: svc}
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
	// 获取token和userID
	tokenStr := c.GetHeader("Authorization")
	userID := c.MustGet("user_id").(uint)

	// 更新数据库：登出时间和状态
	now := time.Now()
	if err := h.svc.Logout(c.Request.Context(), userID, now); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "退出登录失败"})
		return
	}

	// 加入黑名单
	claims, _ := jwt.ParseToken(tokenStr)
	blacklistKey := fmt.Sprintf("blacklist:%s", claims.JTI)
	ttl := time.Until(claims.ExpiresAt.Time)
	redisstore.RDB.Set(c, blacklistKey, "1", ttl)

	// 删除 Redis 中的 token
	redisKey := fmt.Sprintf("token:%s", tokenStr)
	redisstore.RDB.Del(c, redisKey)

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
