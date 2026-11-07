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

// CreateGroup 在事务中创建群会话并加入群主和受邀好友。
func (r *Repository) CreateGroup(
	ctx context.Context,
	ownerID uint64,
	name string,
	avatarURL *string,
	memberUserIDs []uint64,
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
		// 受邀成员必须仍是群主的有效好友。前端选择列表只是交互提示，
		// 后端校验才是防止任意用户被批量拉入群聊的权限边界。
		var friendCount int64
		if err := tx.Table("friendships").
			Where(
				"status = ? AND ("+
					"(user_low_id = ? AND user_high_id IN ?) OR "+
					"(user_high_id = ? AND user_low_id IN ?))",
				1,
				ownerID,
				memberUserIDs,
				ownerID,
				memberUserIDs,
			).
			Count(&friendCount).Error; err != nil {
			return fmt.Errorf("校验群成员好友关系: %w", err)
		}
		if friendCount != int64(len(memberUserIDs)) {
			return ErrNotFriend
		}

		if err := tx.Create(result).Error; err != nil {
			return err
		}
		members := make([]conversationMember, 0, len(memberUserIDs)+1)
		members = append(members, conversationMember{
			ConversationID: result.ID,
			UserID:         ownerID,
			Role:           groupOwnerRole,
			JoinedAt:       now,
			UpdatedAt:      now,
		})
		for _, userID := range memberUserIDs {
			members = append(members, conversationMember{
				ConversationID: result.ID,
				UserID:         userID,
				Role:           1,
				JoinedAt:       now,
				UpdatedAt:      now,
			})
		}
		return tx.Create(&members).Error
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
				"c.last_seq, m.last_read_seq, c.updated_at, "+
				"(SELECT COUNT(*) FROM conversation_members AS active_member "+
				"WHERE active_member.conversation_id = c.id "+
				"AND active_member.left_at IS NULL) AS member_count",
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

// ListMembers 返回当前有效成员可查看的会话成员资料。
func (r *Repository) ListMembers(
	ctx context.Context,
	userID uint64,
	conversationID uint64,
) ([]memberRecord, error) {
	var membershipCount int64
	if err := r.db.WithContext(ctx).
		Table("conversation_members").
		Where(
			"conversation_id = ? AND user_id = ? AND left_at IS NULL",
			conversationID,
			userID,
		).
		Count(&membershipCount).Error; err != nil {
		return nil, fmt.Errorf("校验会话成员权限: %w", err)
	}
	if membershipCount != 1 {
		return nil, ErrConversationNotFound
	}

	var records []memberRecord
	if err := r.db.WithContext(ctx).
		Table("conversation_members AS cm").
		Select(
			"cm.user_id, u.username, "+
				"COALESCE(p.nickname, u.username) AS nickname, "+
				"COALESCE(p.avatar_url, '') AS avatar_url, "+
				"cm.role, cm.joined_at",
		).
		Joins("JOIN users AS u ON u.id = cm.user_id").
		Joins("LEFT JOIN user_profiles AS p ON p.user_id = cm.user_id").
		Where(
			"cm.conversation_id = ? AND cm.left_at IS NULL",
			conversationID,
		).
		Order(
			"CASE cm.role WHEN 3 THEN 0 WHEN 2 THEN 1 ELSE 2 END, " +
				"p.nickname ASC, u.username ASC",
		).
		Scan(&records).Error; err != nil {
		return nil, fmt.Errorf("查询会话成员: %w", err)
	}
	return records, nil
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
