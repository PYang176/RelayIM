package friendship

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"relayim/internal/platform/snowflake"
)

// Repository 负责好友关系和申请的数据访问。
type Repository struct {
	db *gorm.DB
}

// NewRepository 创建好友 Repository。
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// searchUsers 按条件搜索用户
func (r *Repository) searchUsers(
	ctx context.Context,
	userID uint64,
	query string,
) ([]UserResult, error) {
	like := "%" + query + "%"
	var records []userSearchRecord
	if err := r.db.WithContext(ctx).
		Table("users AS u").
		Select("u.id, u.username, p.nickname, p.avatar_url").
		Joins("JOIN user_profiles AS p ON p.user_id = u.id").
		Where(
			"u.id <> ? AND u.status = 1 AND u.deleted_at IS NULL",
			userID,
		).
		Where(
			"NOT EXISTS (SELECT 1 FROM user_blocks AS ub "+
				"WHERE (ub.blocker_id = ? AND ub.blocked_id = u.id) "+
				"OR (ub.blocker_id = u.id AND ub.blocked_id = ?))",
			userID,
			userID,
		).
		Where("(u.username LIKE ? OR p.nickname LIKE ?)", like, like).
		Order("u.username ASC").
		Limit(20).
		Scan(&records).Error; err != nil {
		return nil, fmt.Errorf("搜索用户: %w", err)
	}

	results := make([]UserResult, 0, len(records))
	for _, record := range records {
		results = append(results, UserResult{
			ID:        record.ID,
			Username:  record.Username,
			Nickname:  record.Nickname,
			AvatarURL: stringValue(record.AvatarURL),
		})
	}
	return results, nil
}

// sendRequest 创建好友申请并处理重复申请、拉黑和唯一约束
func (r *Repository) sendRequest(
	ctx context.Context,
	userID uint64,
	targetUserID uint64,
	remark string,
) error {
	lowID, highID := orderedUsers(userID, targetUserID)
	now := time.Now().UTC()

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var targetCount int64
		if err := tx.Table("users").
			Where("id = ? AND status = 1 AND deleted_at IS NULL", targetUserID).
			Count(&targetCount).Error; err != nil {
			return fmt.Errorf("校验好友申请目标: %w", err)
		}
		if targetCount != 1 {
			return ErrUserNotFound
		}
		blocked, err := isBlockedPair(tx, userID, targetUserID)
		if err != nil {
			return err
		}
		if blocked {
			return ErrUserBlocked
		}

		var existingFriendship friendship
		err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where(
				"user_low_id = ? AND user_high_id = ?",
				lowID,
				highID,
			).
			First(&existingFriendship).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("查询好友关系: %w", err)
		}
		if err == nil && existingFriendship.Status == friendshipStatusNormal {
			return ErrAlreadyFriends
		}

		var existingRequest friendRequest
		err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where(
				"user_low_id = ? AND user_high_id = ?",
				lowID,
				highID,
			).
			First(&existingRequest).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("查询好友申请: %w", err)
		}
		if err == nil {
			if existingRequest.Status == requestStatusPending {
				return ErrRequestPending
			}
			return tx.Model(&friendRequest{}).
				Where("id = ?", existingRequest.ID).
				Updates(map[string]any{
					"requester_id": userID,
					"status":       requestStatusPending,
					"remark":       nullableString(remark),
					"created_at":   now,
					"updated_at":   now,
					"responded_at": nil,
				}).Error
		}

		request := friendRequest{
			ID:          uint64(snowflake.Generate()),
			UserLowID:   lowID,
			UserHighID:  highID,
			RequesterID: userID,
			Status:      requestStatusPending,
			Remark:      nullableString(remark),
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		if err := tx.Create(&request).Error; err != nil {
			return err
		}
		return nil
	})
	if isDuplicateKey(err) {
		return ErrRequestPending
	}
	return err
}

// respondToRequest 处理好友申请审批并更新申请与好友关系状态
func (r *Repository) respondToRequest(
	ctx context.Context,
	userID uint64,
	requestID uint64,
	accept bool,
) error {
	now := time.Now().UTC()
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var request friendRequest
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", requestID).
			First(&request).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrRequestNotFound
			}
			return fmt.Errorf("查询好友申请: %w", err)
		}
		if request.RequesterID == userID {
			return ErrRequestForbidden
		}
		if request.UserLowID != userID && request.UserHighID != userID {
			return ErrRequestForbidden
		}
		if request.Status != requestStatusPending {
			return ErrRequestNotFound
		}

		status := requestStatusRejected
		if accept {
			status = requestStatusAccepted
		}
		if err := tx.Model(&friendRequest{}).
			Where("id = ?", request.ID).
			Updates(map[string]any{
				"status":       status,
				"responded_at": now,
				"updated_at":   now,
			}).Error; err != nil {
			return fmt.Errorf("更新好友申请: %w", err)
		}
		if !accept {
			return nil
		}
		blocked, err := isBlockedPair(
			tx,
			request.UserLowID,
			request.UserHighID,
		)
		if err != nil {
			return err
		}
		if blocked {
			return ErrUserBlocked
		}

		var relation friendship
		err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where(
				"user_low_id = ? AND user_high_id = ?",
				request.UserLowID,
				request.UserHighID,
			).
			First(&relation).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			relation = friendship{
				ID:         uint64(snowflake.Generate()),
				UserLowID:  request.UserLowID,
				UserHighID: request.UserHighID,
				Status:     friendshipStatusNormal,
				CreatedAt:  now,
				UpdatedAt:  now,
			}
			return tx.Create(&relation).Error
		}
		if err != nil {
			return fmt.Errorf("查询好友关系: %w", err)
		}
		return tx.Model(&friendship{}).
			Where("id = ?", relation.ID).
			Updates(map[string]any{
				"status":     friendshipStatusNormal,
				"updated_at": now,
			}).Error
	})
}

// listFriends 查询当前用户的好友关系并应用游标分页
func (r *Repository) listFriends(
	ctx context.Context,
	userID uint64,
	limit int,
	beforeID uint64,
) (FriendPage, error) {
	lowRecords, err := r.listFriendsBySide(
		ctx,
		userID,
		limit,
		beforeID,
		true,
	)
	if err != nil {
		return FriendPage{}, err
	}
	highRecords, err := r.listFriendsBySide(
		ctx,
		userID,
		limit,
		beforeID,
		false,
	)
	if err != nil {
		return FriendPage{}, err
	}
	records := append(lowRecords, highRecords...)
	sort.Slice(records, func(left int, right int) bool {
		return records[left].FriendshipID > records[right].FriendshipID
	})

	hasMore := len(records) > limit
	if hasMore {
		records = records[:limit]
	}
	results := make([]FriendResult, 0, len(records))
	for _, record := range records {
		results = append(results, FriendResult{
			FriendshipID: record.FriendshipID,
			User: UserResult{
				ID:        record.FriendUserID,
				Username:  record.Username,
				Nickname:  record.Nickname,
				AvatarURL: stringValue(record.AvatarURL),
			},
			Remark:      record.Remark,
			BlockedByMe: record.BlockedByMe,
		})
	}
	page := FriendPage{
		Friends: results,
		HasMore: hasMore,
	}
	if hasMore && len(results) > 0 {
		page.NextCursor = results[len(results)-1].FriendshipID
	}
	return page, nil
}

// listFriendsBySide 按好友关系的任一侧查询联系人并应用分页
func (r *Repository) listFriendsBySide(
	ctx context.Context,
	userID uint64,
	limit int,
	beforeID uint64,
	lowSide bool,
) ([]friendRecord, error) {
	peerJoin := "JOIN users AS u ON u.id = f.user_high_id"
	sideCondition := "f.user_low_id = ?"
	tableName := "friendships AS f USE INDEX (idx_friendships_low_status_id)"
	if !lowSide {
		peerJoin = "JOIN users AS u ON u.id = f.user_low_id"
		sideCondition = "f.user_high_id = ?"
		tableName = "friendships AS f USE INDEX (idx_friendships_high_status_id)"
	}

	var records []friendRecord
	query := r.db.WithContext(ctx).
		Table(tableName).
		Select(
			"f.id AS friendship_id, u.id AS friend_user_id, "+
				"u.username, p.nickname, p.avatar_url, "+
				"COALESCE(r.remark, '') AS remark, "+
				"(ub.blocker_id IS NOT NULL) AS blocked_by_me",
		).
		Joins(peerJoin).
		Joins("JOIN user_profiles AS p ON p.user_id = u.id").
		Joins(
			"LEFT JOIN friend_remarks AS r "+
				"ON r.friendship_id = f.id AND r.user_id = ?",
			userID,
		).
		Joins(
			"LEFT JOIN user_blocks AS ub "+
				"ON ub.blocker_id = ? AND ub.blocked_id = u.id",
			userID,
		).
		Where(
			"f.status = ? AND "+sideCondition,
			friendshipStatusNormal,
			userID,
		)
	if beforeID > 0 {
		query = query.Where("f.id < ?", beforeID)
	}
	if err := query.
		Order("f.id DESC").
		Limit(limit + 1).
		Scan(&records).Error; err != nil {
		return nil, fmt.Errorf("查询好友列表: %w", err)
	}
	return records, nil
}

// listBlockedUsers 查询拉黑用户并保持权限和分页语义
func (r *Repository) listBlockedUsers(
	ctx context.Context,
	userID uint64,
	limit int,
	beforeID uint64,
) (BlockedUserPage, error) {
	var records []blockedUserRecord
	query := r.db.WithContext(ctx).
		Table("user_blocks AS b USE INDEX (PRIMARY)").
		Select(
			"u.id AS user_id, u.username, p.nickname, "+
				"p.avatar_url, b.created_at",
		).
		Joins("JOIN users AS u ON u.id = b.blocked_id").
		Joins("JOIN user_profiles AS p ON p.user_id = u.id").
		Where("b.blocker_id = ?", userID)
	if beforeID > 0 {
		query = query.Where("b.blocked_id < ?", beforeID)
	}
	if err := query.
		Order("b.blocked_id DESC").
		Limit(limit + 1).
		Scan(&records).Error; err != nil {
		return BlockedUserPage{}, fmt.Errorf("查询拉黑列表: %w", err)
	}

	hasMore := len(records) > limit
	if hasMore {
		records = records[:limit]
	}
	results := make([]BlockedUserResult, 0, len(records))
	for _, record := range records {
		results = append(results, BlockedUserResult{
			User: UserResult{
				ID:        record.UserID,
				Username:  record.Username,
				Nickname:  record.Nickname,
				AvatarURL: stringValue(record.AvatarURL),
			},
			CreatedAt: record.CreatedAt,
		})
	}
	page := BlockedUserPage{
		BlockedUsers: results,
		HasMore:      hasMore,
	}
	if hasMore && len(results) > 0 {
		page.NextCursor = results[len(results)-1].User.ID
	}
	return page, nil
}

// blockUser 创建拉黑关系，并更新双方后续可交互状态
func (r *Repository) blockUser(
	ctx context.Context,
	userID uint64,
	targetUserID uint64,
) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var targetCount int64
		if err := tx.Table("users").
			Where(
				"id = ? AND status = 1 AND deleted_at IS NULL",
				targetUserID,
			).
			Count(&targetCount).Error; err != nil {
			return fmt.Errorf("校验拉黑目标: %w", err)
		}
		if targetCount != 1 {
			return ErrUserNotFound
		}

		record := userBlock{
			BlockerID: userID,
			BlockedID: targetUserID,
			CreatedAt: time.Now().UTC(),
		}
		return tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "blocker_id"},
				{Name: "blocked_id"},
			},
			DoNothing: true,
		}).Create(&record).Error
	})
}

// unblockUser 删除拉黑关系，并恢复双方可交互状态
func (r *Repository) unblockUser(
	ctx context.Context,
	userID uint64,
	targetUserID uint64,
) error {
	return r.db.WithContext(ctx).
		Where(
			"blocker_id = ? AND blocked_id = ?",
			userID,
			targetUserID,
		).
		Delete(&userBlock{}).Error
}

// listRequests 查询好友申请列表并应用游标分页
func (r *Repository) listRequests(
	ctx context.Context,
	userID uint64,
	received bool,
	limit int,
	beforeID uint64,
) (RequestPage, error) {
	var records []requestRecord
	query := r.db.WithContext(ctx).Table("friend_requests AS fr")
	if received {
		query = query.
			Select(
				"fr.id, fr.requester_id, u.id AS user_id, "+
					"u.username, p.nickname, p.avatar_url, "+
					"fr.status, fr.remark, fr.created_at",
			).
			Joins("JOIN users AS u ON u.id = fr.requester_id").
			Joins("JOIN user_profiles AS p ON p.user_id = u.id").
			Where(
				"fr.status = ? AND fr.requester_id <> ? "+
					"AND (fr.user_low_id = ? OR fr.user_high_id = ?)",
				requestStatusPending,
				userID,
				userID,
				userID,
			)
	} else {
		query = query.
			Select(
				"fr.id, fr.requester_id, u.id AS user_id, "+
					"u.username, p.nickname, p.avatar_url, "+
					"fr.status, fr.remark, fr.created_at",
			).
			Joins(
				"JOIN users AS u ON u.id = CASE "+
					"WHEN fr.user_low_id = ? THEN fr.user_high_id "+
					"ELSE fr.user_low_id END",
				userID,
			).
			Joins("JOIN user_profiles AS p ON p.user_id = u.id").
			Where(
				"fr.requester_id = ? AND "+
					"(fr.user_low_id = ? OR fr.user_high_id = ?)",
				userID,
				userID,
				userID,
			)
	}
	if beforeID > 0 {
		query = query.Where("fr.id < ?", beforeID)
	}
	if err := query.
		Order("fr.id DESC").
		Limit(limit + 1).
		Scan(&records).Error; err != nil {
		return RequestPage{}, fmt.Errorf("查询好友申请: %w", err)
	}

	hasMore := len(records) > limit
	if hasMore {
		records = records[:limit]
	}
	results := make([]RequestResult, 0, len(records))
	for _, record := range records {
		results = append(results, RequestResult{
			ID: record.ID,
			User: UserResult{
				ID:        record.UserID,
				Username:  record.Username,
				Nickname:  record.Nickname,
				AvatarURL: stringValue(record.AvatarURL),
			},
			Status:    record.Status,
			Remark:    stringValue(record.Remark),
			CreatedAt: record.CreatedAt,
		})
	}
	page := RequestPage{
		Requests: results,
		HasMore:  hasMore,
	}
	if hasMore && len(results) > 0 {
		page.NextCursor = results[len(results)-1].ID
	}
	return page, nil
}

// setRemark 设置备注并校验边界
func (r *Repository) setRemark(
	ctx context.Context,
	userID uint64,
	friendUserID uint64,
	remark string,
) error {
	lowID, highID := orderedUsers(userID, friendUserID)
	now := time.Now().UTC()
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var relation friendship
		if err := tx.Where(
			"user_low_id = ? AND user_high_id = ? AND status = ?",
			lowID,
			highID,
			friendshipStatusNormal,
		).First(&relation).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrFriendshipNotFound
			}
			return fmt.Errorf("查询好友关系: %w", err)
		}
		if remark == "" {
			return tx.Where(
				"friendship_id = ? AND user_id = ?",
				relation.ID,
				userID,
			).Delete(&friendRemark{}).Error
		}

		record := friendRemark{
			FriendshipID: relation.ID,
			UserID:       userID,
			Remark:       remark,
			CreatedAt:    now,
			UpdatedAt:    now,
		}
		return tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "friendship_id"},
				{Name: "user_id"},
			},
			DoUpdates: clause.Assignments(map[string]any{
				"remark":     remark,
				"updated_at": now,
			}),
		}).Create(&record).Error
	})
}

// deleteFriend 删除好友并遵循关联约束
func (r *Repository) deleteFriend(
	ctx context.Context,
	userID uint64,
	friendUserID uint64,
) error {
	lowID, highID := orderedUsers(userID, friendUserID)
	now := time.Now().UTC()
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var relation friendship
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where(
				"user_low_id = ? AND user_high_id = ? AND status = ?",
				lowID,
				highID,
				friendshipStatusNormal,
			).
			First(&relation).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrFriendshipNotFound
			}
			return fmt.Errorf("查询好友关系: %w", err)
		}
		if err := tx.Model(&friendship{}).
			Where("id = ?", relation.ID).
			Updates(map[string]any{
				"status":     friendshipStatusDeleted,
				"updated_at": now,
			}).Error; err != nil {
			return fmt.Errorf("删除好友关系: %w", err)
		}
		if err := tx.Where("friendship_id = ?", relation.ID).
			Delete(&friendRemark{}).Error; err != nil {
			return fmt.Errorf("删除好友备注: %w", err)
		}
		return nil
	})
}

// orderedUsers 按稳定顺序返回一对用户 ID，供数据库唯一键和查询使用
func orderedUsers(first uint64, second uint64) (uint64, uint64) {
	if first < second {
		return first, second
	}
	return second, first
}

// isBlockedPair 判断两个用户之间是否存在任一方向的拉黑关系
func isBlockedPair(db *gorm.DB, first uint64, second uint64) (bool, error) {
	var count int64
	if err := db.Table("user_blocks").
		Where(
			"(blocker_id = ? AND blocked_id = ?) OR "+
				"(blocker_id = ? AND blocked_id = ?)",
			first,
			second,
			second,
			first,
		).
		Count(&count).Error; err != nil {
		return false, fmt.Errorf("校验用户拉黑关系: %w", err)
	}
	return count > 0, nil
}

// nullableString 把空字符串转换为数据库可空值
func nullableString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

// stringValue 读取字符串值并去除首尾空白
func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

// isDuplicateKey 判断重复键条件
func isDuplicateKey(err error) bool {
	var mysqlErr *mysql.MySQLError
	return errors.As(err, &mysqlErr) && mysqlErr.Number == 1062
}
