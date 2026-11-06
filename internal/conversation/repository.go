package conversation

import (
	"context"
	"errors"
	"fmt"
	"time"

	"ginChatProject/internal/platform/snowflake"
	"github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
)

// Repository 负责会话与成员的数据库访问。
type Repository struct {
	db *gorm.DB
}

// NewRepository 创建会话 Repository。
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// CreateDirect 创建或返回两个用户之间的唯一私聊会话。
// created 仅在本次调用真正创建新会话时为 true；已有会话和并发冲突返回 false。
func (r *Repository) CreateDirect(
	ctx context.Context,
	userID uint64,
	peerUserID uint64,
) (*conversation, bool, error) {
	lowID, highID := orderedUsers(userID, peerUserID)

	existing, err := r.findDirect(ctx, lowID, highID)
	if err == nil {
		return existing, false, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, fmt.Errorf("查询私聊会话: %w", err)
	}

	now := time.Now().UTC()
	result := &conversation{
		ID:               uint64(snowflake.Generate()),
		ConversationType: conversationTypeDirect,
		DirectLowUserID:  &lowID,
		DirectHighUserID: &highID,
		Status:           conversationStatusOpen,
		CreatedAt:        now,
		UpdatedAt:        now,
	}

	// 会话和两名成员必须同时提交，否则会出现没有任何成员的可访问会话。
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(result).Error; err != nil {
			return err
		}
		members := []conversationMember{
			{
				ConversationID: result.ID,
				UserID:         userID,
				Role:           1,
				JoinedAt:       now,
				UpdatedAt:      now,
			},
			{
				ConversationID: result.ID,
				UserID:         peerUserID,
				Role:           1,
				JoinedAt:       now,
				UpdatedAt:      now,
			},
		}
		return tx.Create(&members).Error
	})
	if err == nil {
		return result, true, nil
	}

	// 并发创建相同私聊时唯一索引会兜底。重新读取先提交的会话并返回同一 ID。
	if isDuplicateKey(err) {
		existing, findErr := r.findDirect(ctx, lowID, highID)
		if findErr != nil {
			return nil, false, fmt.Errorf("读取并发私聊会话: %w", findErr)
		}
		return existing, false, nil
	}
	if isForeignKeyError(err) {
		return nil, false, ErrUserNotFound
	}
	return nil, false, err
}

// CreateGroup 在事务中创建群会话并加入群主。
func (r *Repository) CreateGroup(
	ctx context.Context,
	ownerID uint64,
	name string,
	avatarURL *string,
) (*conversation, error) {
	now := time.Now().UTC()
	result := &conversation{
		ID:               uint64(snowflake.Generate()),
		ConversationType: conversationTypeGroup,
		OwnerID:          &ownerID,
		Name:             &name,
		AvatarURL:        avatarURL,
		Status:           conversationStatusOpen,
		CreatedAt:        now,
		UpdatedAt:        now,
	}

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(result).Error; err != nil {
			return err
		}
		owner := conversationMember{
			ConversationID: result.ID,
			UserID:         ownerID,
			Role:           groupOwnerRole,
			JoinedAt:       now,
			UpdatedAt:      now,
		}
		return tx.Create(&owner).Error
	})
	if err != nil {
		if isForeignKeyError(err) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("创建群会话: %w", err)
	}
	return result, nil
}

// ListForUser 返回用户当前有效会话，按最近活动时间倒序。
func (r *Repository) ListForUser(
	ctx context.Context,
	userID uint64,
) ([]conversationSummary, error) {
	var results []conversationSummary
	err := r.db.WithContext(ctx).
		Table("conversation_members AS m").
		Select(
			"c.id, c.conversation_type, c.name, c.avatar_url, "+
				"c.last_seq, m.last_read_seq, c.updated_at",
		).
		Joins("JOIN conversations AS c ON c.id = m.conversation_id").
		Where("m.user_id = ? AND m.left_at IS NULL AND c.status = ?",
			userID,
			conversationStatusOpen,
		).
		Order("c.updated_at DESC").
		Limit(100).
		Scan(&results).Error
	if err != nil {
		return nil, fmt.Errorf("查询用户会话: %w", err)
	}
	return results, nil
}

func (r *Repository) findDirect(
	ctx context.Context,
	lowID uint64,
	highID uint64,
) (*conversation, error) {
	var result conversation
	err := r.db.WithContext(ctx).
		Where(
			"conversation_type = ? AND direct_low_user_id = ? AND direct_high_user_id = ?",
			conversationTypeDirect,
			lowID,
			highID,
		).
		First(&result).Error
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func orderedUsers(first uint64, second uint64) (uint64, uint64) {
	if first < second {
		return first, second
	}
	return second, first
}

func isDuplicateKey(err error) bool {
	var mysqlErr *mysql.MySQLError
	return errors.As(err, &mysqlErr) && mysqlErr.Number == 1062
}

func isForeignKeyError(err error) bool {
	var mysqlErr *mysql.MySQLError
	return errors.As(err, &mysqlErr) && mysqlErr.Number == 1452
}
