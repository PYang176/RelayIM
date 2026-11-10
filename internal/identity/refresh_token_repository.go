package identity

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// createRefreshToken 创建刷新令牌并在事务内维护关联状态
func (r *Repository) createRefreshToken(
	ctx context.Context,
	record *refreshToken,
) error {
	if err := r.db.WithContext(ctx).Create(record).Error; err != nil {
		return fmt.Errorf("保存 Refresh Token: %w", err)
	}
	return nil
}

// rotateRefreshToken 在事务内轮换刷新令牌，防止旧令牌重复使用
func (r *Repository) rotateRefreshToken(
	ctx context.Context,
	oldHash string,
	next *refreshToken,
	now time.Time,
) (uint64, uint64, error) {
	var userID uint64
	var deviceID uint64
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current refreshToken
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where(
				"token_hash = ? AND revoked_at IS NULL",
				oldHash,
			).
			First(&current).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrInvalidRefreshToken
			}
			return fmt.Errorf("查询 Refresh Token: %w", err)
		}
		if !current.ExpiresAt.After(now) {
			return ErrInvalidRefreshToken
		}

		var deviceRecord device
		if err := tx.Where(
			"id = ? AND user_id = ? AND revoked_at IS NULL",
			current.DeviceID,
			current.UserID,
		).First(&deviceRecord).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrInvalidRefreshToken
			}
			return fmt.Errorf("校验 Refresh Token 设备: %w", err)
		}
		if err := tx.Model(&refreshToken{}).
			Where("id = ? AND revoked_at IS NULL", current.ID).
			Update("revoked_at", now).Error; err != nil {
			return fmt.Errorf("撤销旧 Refresh Token: %w", err)
		}

		next.UserID = current.UserID
		next.DeviceID = current.DeviceID
		if err := tx.Create(next).Error; err != nil {
			return fmt.Errorf("保存轮换后的 Refresh Token: %w", err)
		}
		userID = current.UserID
		deviceID = current.DeviceID
		return nil
	})
	return userID, deviceID, err
}

// revokeRefreshToken 撤销指定刷新令牌并写入撤销时间
func (r *Repository) revokeRefreshToken(
	ctx context.Context,
	tokenHash string,
	now time.Time,
) error {
	if err := r.db.WithContext(ctx).
		Model(&refreshToken{}).
		Where("token_hash = ? AND revoked_at IS NULL", tokenHash).
		Update("revoked_at", now).Error; err != nil {
		return fmt.Errorf("撤销 Refresh Token: %w", err)
	}
	return nil
}
