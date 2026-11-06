package identity

import (
	"context"
	"errors"
	"fmt"

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

func isDuplicateKey(err error) bool {
	// GORM 会包装事务内部错误，errors.As 用于提取底层 MySQL 错误。
	var mysqlErr *mysql.MySQLError
	return errors.As(err, &mysqlErr) && mysqlErr.Number == 1062
}
