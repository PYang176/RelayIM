package jwt

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"ginChatProject/internal/config"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var (
	JwtCfg = config.JWTConfig{}
	//jwtSecret = []byte("your-secret-key") //  放配置文件
)

type Claims struct {
	// UserID 是 Snowflake 用户 ID，使用 uint64 避免多实例 ID 被降窄。
	UserID uint64 `json:"user_id"`
	// JTI 是令牌唯一标识，用于 Redis 黑名单撤销。
	JTI string `json:"jti"` // 新增唯一令牌标识
	jwt.RegisteredClaims
}

// GenerateToken 生成 JWT
func GenerateToken(userID uint64, expireHours int) (string, error) {
	// 生成随机JTI
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("生成 JWT JTI 失败: %w", err)
	}
	jti := base64.StdEncoding.EncodeToString(b)

	claims := Claims{
		UserID: userID,
		JTI:    jti,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour * time.Duration(expireHours))),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Subject:   strconv.FormatUint(userID, 10),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(JwtCfg.Secret))
}

// ParseToken 解析 JWT
func ParseToken(tokenStr string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(
		tokenStr,
		&Claims{},
		func(t *jwt.Token) (interface{}, error) {
			if t.Method != jwt.SigningMethodHS256 {
				return nil, errors.New("不支持的 JWT 签名算法")
			}
			return []byte(JwtCfg.Secret), nil
		},
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithExpirationRequired(),
	)
	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token")
	}
	if claims.UserID == 0 ||
		claims.Subject != strconv.FormatUint(claims.UserID, 10) {
		return nil, errors.New("invalid token subject")
	}

	return claims, nil
}
