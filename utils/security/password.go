package security

import (
	"golang.org/x/crypto/bcrypt"
)

const (
	DefaultCost = bcrypt.DefaultCost // 10
)

// HashPassword 对明文密码进行加密
func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword(
		[]byte(password),
		DefaultCost,
	)
	return string(bytes), err
}

// CheckPassword 校验明文密码和哈希密码是否匹配
func CheckPassword(plainPassword string, hashedPassword string) bool {
	return bcrypt.CompareHashAndPassword(
		[]byte(hashedPassword),
		[]byte(plainPassword),
	) == nil
}
