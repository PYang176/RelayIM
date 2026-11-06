package jwt

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"ginChatProject/internal/config"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var (
	JwtCfg = config.JWTConfig{}
	//jwtSecret = []byte("your-secret-key") //  放配置文件
)

type Claims struct {
	UserID uint   `json:"user_id"`
	JTI    string `json:"jti"` // 新增唯一令牌标识
	jwt.RegisteredClaims
}

// GenerateToken 生成 JWT
func GenerateToken(userID uint, expireHours int) (string, error) {
	// 生成随机JTI
	b := make([]byte, 32)
	rand.Read(b)
	jti := base64.StdEncoding.EncodeToString(b)

	//fmt.Println("生成token的过期时间为------", expireHours)
	//fmt.Println("配置生成token的过期时间为------", JwtCfg.ExpireHours)
	claims := Claims{
		UserID: userID,
		JTI:    jti,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour * time.Duration(expireHours))),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	//fmt.Println("token密钥--------", JwtCfg.Secret)
	return token.SignedString([]byte(JwtCfg.Secret))
}

// ParseToken 解析 JWT
func ParseToken(tokenStr string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(
		tokenStr,
		&Claims{},
		func(t *jwt.Token) (interface{}, error) {
			return []byte(JwtCfg.Secret), nil
		},
	)
	//fmt.Println("解析时候出错啦----", err)
	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token")
	}

	return claims, nil
}
