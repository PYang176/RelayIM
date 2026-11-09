package identity

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
)

// Repository 负责用户和用户资料的数据库访问。
type Repository struct {
	db *gorm.DB
}

// NewRepository 创建用户 Repository。
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// CreateUser 在同一事务中创建登录身份和公开资料。
func (r *Repository) CreateUser(
	ctx context.Context,
	userRecord *user,
	profile *userProfile,
) error {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// users 先插入，因为 user_profiles.user_id 依赖该主键。

		if err := tx.Create(userRecord).Error; err != nil {
			return fmt.Errorf("创建用户身份: %w", err)
		}

		// 资料创建失败时回滚用户，避免出现没有 profile 的半成品用户。

		if err := tx.Create(profile).Error; err != nil {
			return fmt.Errorf("创建用户资料: %w", err)
		}
		return nil
	})
	if err != nil {
		// 并发注册相同用户名时，唯一索引会返回 MySQL 1062。
		// 将其转换为稳定业务错误，Handler 不需要理解数据库错误码。

		if isDuplicateKey(err) {
			return ErrUsernameExists
		}
		return err
	}
	return nil
}

// FindByUsername 根据唯一用户名查询登录身份。
func (r *Repository) FindByUsername(ctx context.Context, username string) (*user, error) {
	var result user
	err := r.db.WithContext(ctx).
		Where("username = ? AND deleted_at IS NULL", username).
		First(&result).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrInvalidCredentials
	}
	if err != nil {
		return nil, fmt.Errorf("查询用户: %w", err)
	}
	return &result, nil
}

// FindByID 根据 Snowflake ID 查询用户和公开资料。
func (r *Repository) FindByID(ctx context.Context, userID uint64) (*user, *userProfile, error) {
	var userRecord user
	if err := r.db.WithContext(ctx).
		Where("id = ? AND deleted_at IS NULL", userID).
		First(&userRecord).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, ErrUserNotFound
		}
		return nil, nil, fmt.Errorf("查询用户: %w", err)
	}

	var profile userProfile
	if err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		First(&profile).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, ErrUserNotFound
		}
		return nil, nil, fmt.Errorf("查询用户资料: %w", err)
	}
	return &userRecord, &profile, nil
}

// UpdateProfile 更新用户公开资料；空头像或简介会写回 NULL。
func (r *Repository) UpdateProfile(
	ctx context.Context,
	userID uint64,
	nickname string,
	avatarURL *string,
	bio string,
	updatedAt time.Time,
) error {
	updates := map[string]any{
		"nickname":   nickname,
		"bio":        nullableString(bio),
		"updated_at": updatedAt,
	}
	if avatarURL != nil {
		updates["avatar_url"] = nullableString(*avatarURL)
	}
	result := r.db.WithContext(ctx).
		Model(&userProfile{}).
		Where("user_id = ?", userID).
		Updates(updates)
	if result.Error != nil {
		return fmt.Errorf("更新用户资料: %w", result.Error)
	}
	if result.RowsAffected > 0 {
		return nil
	}

	// MySQL 对“写入相同值”可能返回 0 行。再次确认资料存在，
	// 避免把幂等编辑误报为用户不存在。
	var count int64
	if err := r.db.WithContext(ctx).
		Model(&userProfile{}).
		Where("user_id = ?", userID).
		Count(&count).Error; err != nil {
		return fmt.Errorf("确认用户资料存在: %w", err)
	}
	if count != 1 {
		return ErrUserNotFound
	}
	return nil
}

// UpdateAvatar 原子更新头像对象键和对外展示地址。
func (r *Repository) UpdateAvatar(
	ctx context.Context,
	userID uint64,
	objectKey *string,
	avatarURL string,
	updatedAt time.Time,
) error {
	result := r.db.WithContext(ctx).
		Model(&userProfile{}).
		Where("user_id = ?", userID).
		Updates(map[string]any{
			"avatar_object_key": objectKey,
			"avatar_url":        nullableString(avatarURL),
			"updated_at":        updatedAt,
		})
	if result.Error != nil {
		return fmt.Errorf("更新用户头像: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		var count int64
		if err := r.db.WithContext(ctx).
			Model(&userProfile{}).
			Where("user_id = ?", userID).
			Count(&count).Error; err != nil {
			return fmt.Errorf("确认用户头像资料存在: %w", err)
		}
		if count != 1 {
			return ErrUserNotFound
		}
	}
	return nil
}

// FindActiveDevice 查询属于指定用户且未被撤销的设备。
func (r *Repository) FindActiveDevice(
	ctx context.Context,
	userID uint64,
	deviceID uint64,
) (*device, error) {
	var result device
	err := r.db.WithContext(ctx).
		Where(
			"id = ? AND user_id = ? AND revoked_at IS NULL",
			deviceID,
			userID,
		).
		First(&result).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrDeviceNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("查询用户设备: %w", err)
	}
	return &result, nil
}

// CreateDevice 保存新的用户设备。
func (r *Repository) CreateDevice(ctx context.Context, record *device) error {
	if err := r.db.WithContext(ctx).Create(record).Error; err != nil {
		return fmt.Errorf("创建设备: %w", err)
	}
	return nil
}

// TouchDevice 更新设备最近活跃时间。
func (r *Repository) TouchDevice(
	ctx context.Context,
	deviceID uint64,
	seenAt time.Time,
) error {
	result := r.db.WithContext(ctx).
		Model(&device{}).
		Where("id = ? AND revoked_at IS NULL", deviceID).
		Update("last_seen_at", seenAt)
	if result.Error != nil {
		return fmt.Errorf("更新设备活跃时间: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return ErrDeviceNotFound
	}
	return nil
}

func isDuplicateKey(err error) bool {
	// GORM 会包装事务内部错误，errors.As 用于提取底层 MySQL 错误。
	var mysqlErr *mysql.MySQLError
	return errors.As(err, &mysqlErr) && mysqlErr.Number == 1062
}

func nullableString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
