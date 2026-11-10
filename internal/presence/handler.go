package presence

import (
	"context"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"relayim/internal/middleware"
)

const maxPresenceBodyBytes = 16 << 10

// Querier 是 Handler 依赖的最小在线状态能力。
type Querier interface {
	Query(ctx context.Context, userIDs []uint64) ([]Status, error)
}

// Handler 处理在线状态查询。
type Handler struct {
	querier Querier
}

// NewHandler 创建在线状态 Handler。
func NewHandler(querier Querier) *Handler {
	return &Handler{querier: querier}
}

type queryRequest struct {
	// UserIDs 是待查询用户 Snowflake ID 字符串。
	UserIDs []string `json:"user_ids"`
}

// Query 处理 POST /api/v1/presence/query。
func (h *Handler) Query(c *gin.Context) {
	if _, ok := middleware.CurrentUserID(c); !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "登录用户信息无效"})
		return
	}
	c.Request.Body = http.MaxBytesReader(
		c.Writer,
		c.Request.Body,
		maxPresenceBodyBytes,
	)
	var request queryRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数格式错误"})
		return
	}
	userIDs := make([]uint64, 0, len(request.UserIDs))
	for _, rawID := range request.UserIDs {
		userID, err := strconv.ParseUint(rawID, 10, 64)
		if err != nil || userID == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "user_ids 格式错误"})
			return
		}
		userIDs = append(userIDs, userID)
	}
	results, err := h.querier.Query(c.Request.Context(), userIDs)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "在线状态服务暂不可用",
		})
		return
	}
	response := make([]gin.H, 0, len(results))
	for _, result := range results {
		response = append(response, gin.H{
			"user_id": strconv.FormatUint(result.UserID, 10),
			"online":  result.Online,
		})
	}
	c.JSON(http.StatusOK, gin.H{"presence": response})
}
