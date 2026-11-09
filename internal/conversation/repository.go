package conversation

import (
	"context"
	"errors"
	"fmt"
	"time"

	"ginChatProject/internal/platform/snowflake"
	"github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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
	blocked, err := hasBlockedPair(ctx, r.db, userID, peerUserID)
	if err != nil {
		return nil, false, err
	}
	if blocked {
		return nil, false, ErrUserBlocked
	}

	existing, err := r.findDirect(ctx, lowID, highID)
	if err == nil {
		title, titleErr := r.userDisplayName(ctx, peerUserID)
		if titleErr != nil {
			return nil, false, titleErr
		}
		existing.Title = title
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
		title, titleErr := r.userDisplayName(ctx, peerUserID)
		if titleErr != nil {
			return nil, false, titleErr
		}
		result.Title = title
		return result, true, nil
	}

	// 并发创建相同私聊时唯一索引会兜底。重新读取先提交的会话并返回同一 ID。
	if isDuplicateKey(err) {
		existing, findErr := r.findDirect(ctx, lowID, highID)
		if findErr != nil {
			return nil, false, fmt.Errorf("读取并发私聊会话: %w", findErr)
		}
		title, titleErr := r.userDisplayName(ctx, peerUserID)
		if titleErr != nil {
			return nil, false, titleErr
		}
		existing.Title = title
		return existing, false, nil
	}
	if isForeignKeyError(err) {
		return nil, false, ErrUserNotFound
	}
	return nil, false, err
}

// CreateGroup 在事务中创建群会话、加入群主并生成好友邀请。
func (r *Repository) CreateGroup(
	ctx context.Context,
	ownerID uint64,
	name string,
	avatarURL *string,
	memberUserIDs []uint64,
	inviteRemark string,
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
		if err := countFriends(ctx, tx, ownerID, memberUserIDs); err != nil {
			return err
		}

		if err := tx.Create(result).Error; err != nil {
			return err
		}
		if err := tx.Create(&conversationMember{
			ConversationID: result.ID,
			UserID:         ownerID,
			Role:           groupOwnerRole,
			JoinedAt:       now,
			UpdatedAt:      now,
		}).Error; err != nil {
			return fmt.Errorf("加入群主: %w", err)
		}
		_, err := createGroupInvitations(
			tx,
			result.ID,
			ownerID,
			memberUserIDs,
			inviteRemark,
			now,
		)
		return err
	})
	if err != nil {
		if isForeignKeyError(err) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("创建群会话: %w", err)
	}
	result.Title = name
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
				"CASE WHEN c.conversation_type = 1 "+
				"AND c.direct_low_user_id = m.user_id "+
				"THEN c.direct_high_user_id "+
				"WHEN c.conversation_type = 1 "+
				"THEN c.direct_low_user_id ELSE NULL END AS peer_user_id, "+
				"CASE WHEN c.conversation_type = 1 "+
				"THEN COALESCE(dp.nickname, direct_user.username, '私聊') "+
				"ELSE COALESCE(c.name, '群聊') END AS title, "+
				"(SELECT COUNT(*) FROM conversation_members AS active_member "+
				"WHERE active_member.conversation_id = c.id "+
				"AND active_member.left_at IS NULL) AS member_count",
		).
		Joins("JOIN conversations AS c ON c.id = m.conversation_id").
		Joins(
			"LEFT JOIN users AS direct_user ON direct_user.id = CASE "+
				"WHEN c.conversation_type = 1 AND c.direct_low_user_id = m.user_id "+
				"THEN c.direct_high_user_id "+
				"WHEN c.conversation_type = 1 THEN c.direct_low_user_id "+
				"ELSE NULL END",
		).
		Joins(
			"LEFT JOIN user_profiles AS dp ON dp.user_id = direct_user.id",
		).
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

// InviteMembers 在锁住群会话后创建待审批的好友邀请。
func (r *Repository) InviteMembers(
	ctx context.Context,
	actorID uint64,
	conversationID uint64,
	memberUserIDs []uint64,
	inviteRemark string,
) (uint64, error) {
	now := time.Now().UTC()
	var invitedCount uint64
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var conversationRecord conversation
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where(
				"id = ? AND status = ?",
				conversationID,
				conversationStatusOpen,
			).
			First(&conversationRecord).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrConversationNotFound
			}
			return fmt.Errorf("锁定待邀请群会话: %w", err)
		}
		if conversationRecord.ConversationType != conversationTypeGroup {
			return ErrNotGroup
		}

		var actor conversationMember
		if err := tx.Where(
			"conversation_id = ? AND user_id = ? AND left_at IS NULL",
			conversationID,
			actorID,
		).First(&actor).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrConversationNotFound
			}
			return fmt.Errorf("校验邀请人成员身份: %w", err)
		}
		if actor.Role < 2 {
			return ErrPermissionDenied
		}
		if err := countFriends(
			ctx,
			tx,
			actorID,
			memberUserIDs,
		); err != nil {
			return err
		}

		count, err := createGroupInvitations(
			tx,
			conversationID,
			actorID,
			memberUserIDs,
			inviteRemark,
			now,
		)
		if err != nil {
			return err
		}
		invitedCount = uint64(count)
		return nil
	})
	if err != nil {
		if isForeignKeyError(err) {
			return 0, ErrUserNotFound
		}
		return 0, err
	}
	return invitedCount, nil
}

func createGroupInvitations(
	tx *gorm.DB,
	conversationID uint64,
	inviterID uint64,
	inviteeIDs []uint64,
	inviteRemark string,
	now time.Time,
) (int, error) {
	var members []conversationMember
	if err := tx.Where(
		"conversation_id = ? AND user_id IN ?",
		conversationID,
		inviteeIDs,
	).Find(&members).Error; err != nil {
		return 0, fmt.Errorf("查询群成员状态: %w", err)
	}
	memberByUserID := make(map[uint64]conversationMember, len(members))
	for _, member := range members {
		memberByUserID[member.UserID] = member
	}

	var invitations []groupInvitation
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where(
			"conversation_id = ? AND invitee_id IN ?",
			conversationID,
			inviteeIDs,
		).
		Find(&invitations).Error; err != nil {
		return 0, fmt.Errorf("查询群邀请状态: %w", err)
	}
	invitationByInviteeID := make(map[uint64]groupInvitation, len(invitations))
	for _, invitation := range invitations {
		invitationByInviteeID[invitation.InviteeID] = invitation
	}

	expiresAt := now.Add(invitationTTL)
	created := make([]groupInvitation, 0, len(inviteeIDs))
	invitedCount := 0
	for _, inviteeID := range inviteeIDs {
		if member, exists := memberByUserID[inviteeID]; exists &&
			member.LeftAt == nil {
			continue
		}
		existing, exists := invitationByInviteeID[inviteeID]
		if exists &&
			existing.Status == invitationStatusPending &&
			existing.ExpiresAt.After(now) {
			if err := tx.Model(&groupInvitation{}).
				Where("id = ?", existing.ID).
				Updates(map[string]any{
					"inviter_id":    inviterID,
					"invite_remark": inviteRemark,
				}).Error; err != nil {
				return 0, fmt.Errorf("更新待处理群邀请备注: %w", err)
			}
			continue
		}
		if exists {
			if err := tx.Model(&groupInvitation{}).
				Where("id = ?", existing.ID).
				Updates(map[string]any{
					"inviter_id":    inviterID,
					"invite_remark": inviteRemark,
					"status":        invitationStatusPending,
					"created_at":    now,
					"expires_at":    expiresAt,
					"responded_at":  nil,
				}).Error; err != nil {
				return 0, fmt.Errorf("重新激活群邀请: %w", err)
			}
		} else {
			created = append(created, groupInvitation{
				ID:             uint64(snowflake.Generate()),
				ConversationID: conversationID,
				InviterID:      inviterID,
				InviteRemark:   inviteRemark,
				InviteeID:      inviteeID,
				Status:         invitationStatusPending,
				CreatedAt:      now,
				ExpiresAt:      expiresAt,
			})
		}
		invitedCount++
	}
	if len(created) > 0 {
		if err := tx.Create(&created).Error; err != nil {
			return 0, fmt.Errorf("创建群邀请: %w", err)
		}
	}
	return invitedCount, nil
}

func (r *Repository) respondToInvitation(
	ctx context.Context,
	userID uint64,
	invitationID uint64,
	accept bool,
) (*groupInvitation, uint64, error) {
	now := time.Now().UTC()
	var invitation groupInvitation
	var memberCount uint64
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", invitationID).
			First(&invitation).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrInvitationNotFound
			}
			return fmt.Errorf("锁定群邀请: %w", err)
		}
		if invitation.InviteeID != userID {
			return ErrInvitationForbidden
		}
		if invitation.Status == invitationStatusAccepted && accept {
			var activeCount int64
			if err := tx.Model(&conversationMember{}).
				Where(
					"conversation_id = ? AND user_id = ? AND left_at IS NULL",
					invitation.ConversationID,
					userID,
				).
				Count(&activeCount).Error; err != nil {
				return fmt.Errorf("校验已接受邀请成员: %w", err)
			}
			if activeCount == 1 {
				var totalCount int64
				if err := tx.Model(&conversationMember{}).
					Where(
						"conversation_id = ? AND left_at IS NULL",
						invitation.ConversationID,
					).
					Count(&totalCount).Error; err != nil {
					return fmt.Errorf("统计已接受邀请群成员: %w", err)
				}
				memberCount = uint64(totalCount)
				return nil
			}
		}
		if invitation.Status != invitationStatusPending {
			return ErrInvitationHandled
		}
		if !invitation.ExpiresAt.After(now) {
			return ErrInvitationExpired
		}
		if !accept {
			return tx.Model(&groupInvitation{}).
				Where("id = ?", invitation.ID).
				Updates(map[string]any{
					"status":       invitationStatusRejected,
					"responded_at": now,
				}).Error
		}

		var conversationRecord conversation
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where(
				"id = ? AND status = ? AND conversation_type = ?",
				invitation.ConversationID,
				conversationStatusOpen,
				conversationTypeGroup,
			).
			First(&conversationRecord).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrInvitationUnavailable
			}
			return fmt.Errorf("锁定受邀群会话: %w", err)
		}

		var currentCount int64
		if err := tx.Model(&conversationMember{}).
			Where(
				"conversation_id = ? AND left_at IS NULL",
				invitation.ConversationID,
			).
			Count(&currentCount).Error; err != nil {
			return fmt.Errorf("统计受邀群成员: %w", err)
		}
		var existingMember conversationMember
		memberErr := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where(
				"conversation_id = ? AND user_id = ?",
				invitation.ConversationID,
				userID,
			).
			First(&existingMember).Error
		switch {
		case memberErr == nil && existingMember.LeftAt == nil:
			memberCount = uint64(currentCount)
		case memberErr == nil:
			if currentCount >= maxGroupMemberCount {
				return ErrGroupFull
			}
			if err := tx.Model(&conversationMember{}).
				Where(
					"conversation_id = ? AND user_id = ?",
					invitation.ConversationID,
					userID,
				).
				Updates(map[string]any{
					"role":          groupMemberRole,
					"last_read_seq": conversationRecord.LastSeq,
					"joined_at":     now,
					"left_at":       nil,
					"updated_at":    now,
				}).Error; err != nil {
				return fmt.Errorf("恢复受邀群成员: %w", err)
			}
			memberCount = uint64(currentCount + 1)
		case errors.Is(memberErr, gorm.ErrRecordNotFound):
			if currentCount >= maxGroupMemberCount {
				return ErrGroupFull
			}
			if err := tx.Create(&conversationMember{
				ConversationID: invitation.ConversationID,
				UserID:         userID,
				Role:           groupMemberRole,
				LastReadSeq:    conversationRecord.LastSeq,
				JoinedAt:       now,
				UpdatedAt:      now,
			}).Error; err != nil {
				return fmt.Errorf("加入受邀群成员: %w", err)
			}
			memberCount = uint64(currentCount + 1)
		default:
			return fmt.Errorf("查询受邀群成员: %w", memberErr)
		}

		if err := tx.Model(&groupInvitation{}).
			Where("id = ?", invitation.ID).
			Updates(map[string]any{
				"status":       invitationStatusAccepted,
				"responded_at": now,
			}).Error; err != nil {
			return fmt.Errorf("更新群邀请接受状态: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, 0, err
	}
	return &invitation, memberCount, nil
}

func (r *Repository) listReceivedInvitations(
	ctx context.Context,
	userID uint64,
	limit int,
	beforeID uint64,
) (ReceivedInvitationPage, error) {
	now := time.Now().UTC()
	var records []receivedInvitationRecord
	query := r.db.WithContext(ctx).
		Table("group_invitations AS gi").
		Select(
			"gi.id AS invitation_id, gi.conversation_id, "+
				"c.name AS conversation_name, c.avatar_url, "+
				"gi.inviter_id, gi.invite_remark, "+
				"COALESCE(ip.nickname, iu.username) AS inviter_name, "+
				"gi.created_at, gi.expires_at",
		).
		Joins("JOIN conversations AS c ON c.id = gi.conversation_id").
		Joins("JOIN users AS iu ON iu.id = gi.inviter_id").
		Joins("LEFT JOIN user_profiles AS ip ON ip.user_id = iu.id").
		Where(
			"gi.invitee_id = ? AND gi.status = ? "+
				"AND gi.expires_at > ? AND c.status = ?",
			userID,
			invitationStatusPending,
			now,
			conversationStatusOpen,
		)
	if beforeID > 0 {
		query = query.Where("gi.id < ?", beforeID)
	}
	if err := query.
		Order("gi.id DESC").
		Limit(limit + 1).
		Scan(&records).Error; err != nil {
		return ReceivedInvitationPage{}, fmt.Errorf("查询收到群邀请: %w", err)
	}
	hasMore := len(records) > limit
	if hasMore {
		records = records[:limit]
	}
	results := make([]ReceivedInvitationResult, 0, len(records))
	for _, record := range records {
		results = append(results, ReceivedInvitationResult{
			InvitationID:     record.InvitationID,
			ConversationID:   record.ConversationID,
			ConversationName: record.ConversationName,
			AvatarURL:        stringValue(record.AvatarURL),
			InviterID:        record.InviterID,
			InviterName:      record.InviterName,
			InviteRemark:     record.InviteRemark,
			CreatedAt:        record.CreatedAt,
			ExpiresAt:        record.ExpiresAt,
		})
	}
	page := ReceivedInvitationPage{
		Invitations: results,
		HasMore:     hasMore,
	}
	if hasMore && len(results) > 0 {
		page.NextCursor = results[len(results)-1].InvitationID
	}
	return page, nil
}

func (r *Repository) listGroupInvitations(
	ctx context.Context,
	actorID uint64,
	conversationID uint64,
	limit int,
	beforeID uint64,
) (GroupInvitationPage, error) {
	var actor conversationMember
	if err := r.db.WithContext(ctx).
		Where(
			"conversation_id = ? AND user_id = ? AND left_at IS NULL",
			conversationID,
			actorID,
		).
		First(&actor).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return GroupInvitationPage{}, ErrConversationNotFound
		}
		return GroupInvitationPage{}, fmt.Errorf("校验群邀请查看权限: %w", err)
	}
	if actor.Role < groupAdminRole {
		return GroupInvitationPage{}, ErrPermissionDenied
	}

	now := time.Now().UTC()
	var records []groupInvitationRecord
	query := r.db.WithContext(ctx).
		Table(
			"group_invitations AS gi "+
				"USE INDEX (idx_group_invitations_conversation_status_id)",
		).
		Select(
			"gi.id AS invitation_id, gi.invitee_id, "+
				"u.username, p.nickname, p.avatar_url, "+
				"gi.inviter_id, gi.invite_remark, "+
				"gi.created_at, gi.expires_at",
		).
		Joins("JOIN users AS u ON u.id = gi.invitee_id").
		Joins("LEFT JOIN user_profiles AS p ON p.user_id = u.id").
		Where(
			"gi.conversation_id = ? AND gi.status = ? AND gi.expires_at > ?",
			conversationID,
			invitationStatusPending,
			now,
		)
	if beforeID > 0 {
		query = query.Where("gi.id < ?", beforeID)
	}
	if err := query.
		Order("gi.id DESC").
		Limit(limit + 1).
		Scan(&records).Error; err != nil {
		return GroupInvitationPage{}, fmt.Errorf("查询群待处理邀请: %w", err)
	}
	hasMore := len(records) > limit
	if hasMore {
		records = records[:limit]
	}
	results := make([]GroupInvitationResult, 0, len(records))
	for _, record := range records {
		results = append(results, GroupInvitationResult{
			InvitationID: record.InvitationID,
			Invitee: MemberResult{
				UserID:    record.InviteeID,
				Username:  record.Username,
				Nickname:  record.Nickname,
				AvatarURL: stringValue(record.AvatarURL),
			},
			InviterID:    record.InviterID,
			InviteRemark: record.InviteRemark,
			CreatedAt:    record.CreatedAt,
			ExpiresAt:    record.ExpiresAt,
		})
	}
	page := GroupInvitationPage{
		Invitations: results,
		HasMore:     hasMore,
	}
	if hasMore && len(results) > 0 {
		page.NextCursor = results[len(results)-1].InvitationID
	}
	return page, nil
}

// RemoveMember 软移除群成员；普通成员只能把自己的成员记录标记为退出。
func (r *Repository) RemoveMember(
	ctx context.Context,
	actorID uint64,
	conversationID uint64,
	targetUserID uint64,
) error {
	now := time.Now().UTC()
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var conversationRecord conversation
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where(
				"id = ? AND status = ?",
				conversationID,
				conversationStatusOpen,
			).
			First(&conversationRecord).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrConversationNotFound
			}
			return fmt.Errorf("锁定待移除成员群会话: %w", err)
		}
		if conversationRecord.ConversationType != conversationTypeGroup {
			return ErrNotGroup
		}

		var actor conversationMember
		if err := tx.Where(
			"conversation_id = ? AND user_id = ? AND left_at IS NULL",
			conversationID,
			actorID,
		).First(&actor).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrConversationNotFound
			}
			return fmt.Errorf("校验群管理操作人: %w", err)
		}

		if actorID != targetUserID && actor.Role < 2 {
			return ErrPermissionDenied
		}
		if conversationRecord.OwnerID != nil &&
			*conversationRecord.OwnerID == targetUserID {
			if actorID == targetUserID {
				return ErrOwnerCannotLeave
			}
			return ErrPermissionDenied
		}

		var target conversationMember
		if err := tx.Where(
			"conversation_id = ? AND user_id = ? AND left_at IS NULL",
			conversationID,
			targetUserID,
		).First(&target).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrMemberNotFound
			}
			return fmt.Errorf("查询待移除群成员: %w", err)
		}
		if actor.Role == 2 && target.Role != 1 {
			return ErrPermissionDenied
		}
		if err := tx.Model(&conversationMember{}).
			Where(
				"conversation_id = ? AND user_id = ?",
				conversationID,
				targetUserID,
			).
			Updates(map[string]any{
				"left_at":    now,
				"updated_at": now,
			}).Error; err != nil {
			return fmt.Errorf("移除群成员: %w", err)
		}
		return nil
	})
}

// SetMemberRole 由群主设置管理员或取消管理员。
func (r *Repository) SetMemberRole(
	ctx context.Context,
	actorID uint64,
	conversationID uint64,
	targetUserID uint64,
	role int,
) error {
	if role != groupMemberRole && role != groupAdminRole {
		return ErrInvalidInput
	}
	now := time.Now().UTC()
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := lockOwnedGroup(
			ctx,
			tx,
			actorID,
			conversationID,
		); err != nil {
			return err
		}

		// 群主自己的角色由 owner_id 约束，只能通过转让群主改变。
		// 这里拒绝误把群主降为普通成员，避免出现没有群主的群。
		if targetUserID == actorID {
			return ErrPermissionDenied
		}
		var target conversationMember
		if err := tx.Where(
			"conversation_id = ? AND user_id = ? AND left_at IS NULL",
			conversationID,
			targetUserID,
		).First(&target).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrMemberNotFound
			}
			return fmt.Errorf("查询待调整角色成员: %w", err)
		}
		if target.Role == groupOwnerRole {
			return ErrPermissionDenied
		}
		if target.Role == role {
			return nil
		}
		if err := tx.Model(&conversationMember{}).
			Where(
				"conversation_id = ? AND user_id = ? AND left_at IS NULL",
				conversationID,
				targetUserID,
			).
			Updates(map[string]any{
				"role":       role,
				"updated_at": now,
			}).Error; err != nil {
			return fmt.Errorf("更新群成员角色: %w", err)
		}
		return nil
	})
}

// TransferOwner 在同一事务内迁移会话所有者并交换双方角色。
func (r *Repository) TransferOwner(
	ctx context.Context,
	actorID uint64,
	conversationID uint64,
	targetUserID uint64,
) error {
	now := time.Now().UTC()
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := lockOwnedGroup(
			ctx,
			tx,
			actorID,
			conversationID,
		); err != nil {
			return err
		}
		if targetUserID == actorID {
			return ErrInvalidInput
		}

		var target conversationMember
		if err := tx.Where(
			"conversation_id = ? AND user_id = ? AND left_at IS NULL",
			conversationID,
			targetUserID,
		).First(&target).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrMemberNotFound
			}
			return fmt.Errorf("查询群主受让人: %w", err)
		}
		if target.Role == groupOwnerRole {
			return ErrPermissionDenied
		}

		// conversations.owner_id 与两名成员的角色必须一起提交。
		// 否则失败会留下“会话所有者”和成员角色互相矛盾的群。
		if err := tx.Model(&conversation{}).
			Where("id = ? AND status = ?", conversationID, conversationStatusOpen).
			Updates(map[string]any{
				"owner_id":   targetUserID,
				"updated_at": now,
			}).Error; err != nil {
			return fmt.Errorf("更新群主字段: %w", err)
		}
		if err := tx.Model(&conversationMember{}).
			Where(
				"conversation_id = ? AND user_id = ? AND left_at IS NULL",
				conversationID,
				actorID,
			).
			Updates(map[string]any{
				"role":       groupMemberRole,
				"updated_at": now,
			}).Error; err != nil {
			return fmt.Errorf("降级原群主: %w", err)
		}
		if err := tx.Model(&conversationMember{}).
			Where(
				"conversation_id = ? AND user_id = ? AND left_at IS NULL",
				conversationID,
				targetUserID,
			).
			Updates(map[string]any{
				"role":       groupOwnerRole,
				"updated_at": now,
			}).Error; err != nil {
			return fmt.Errorf("升级新群主: %w", err)
		}
		return nil
	})
}

// DissolveGroup 将群会话标记为已解散，成员和消息记录保留用于审计。
func (r *Repository) DissolveGroup(
	ctx context.Context,
	actorID uint64,
	conversationID uint64,
) error {
	now := time.Now().UTC()
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := lockOwnedGroup(
			ctx,
			tx,
			actorID,
			conversationID,
		); err != nil {
			return err
		}

		// 消息发送同样先锁定会话行并检查 status。只要解散事务先提交，
		// 后续发送就会得到“会话已关闭”，不会在已解散群继续产生新消息。
		if err := tx.Model(&conversation{}).
			Where("id = ? AND status = ?", conversationID, conversationStatusOpen).
			Updates(map[string]any{
				"status":     conversationStatusDissolved,
				"updated_at": now,
			}).Error; err != nil {
			return fmt.Errorf("解散群会话: %w", err)
		}
		return nil
	})
}

// UpdateGroupProfile 由群主更新群名称和头像。
func (r *Repository) UpdateGroupProfile(
	ctx context.Context,
	actorID uint64,
	conversationID uint64,
	name string,
	avatarURL string,
) error {
	now := time.Now().UTC()
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := lockOwnedGroup(
			ctx,
			tx,
			actorID,
			conversationID,
		); err != nil {
			return err
		}
		if err := tx.Model(&conversation{}).
			Where("id = ? AND status = ?", conversationID, conversationStatusOpen).
			Updates(map[string]any{
				"name":       name,
				"avatar_url": nullableString(avatarURL),
				"updated_at": now,
			}).Error; err != nil {
			return fmt.Errorf("更新群资料: %w", err)
		}
		return nil
	})
}

// ListMembers 返回当前有效成员可查看的会话成员资料。
func (r *Repository) ListMembers(
	ctx context.Context,
	userID uint64,
	conversationID uint64,
) ([]memberRecord, error) {
	var membershipCount int64
	if err := r.db.WithContext(ctx).
		Table("conversation_members AS cm").
		Joins("JOIN conversations AS c ON c.id = cm.conversation_id").
		Where(
			"cm.conversation_id = ? AND cm.user_id = ? "+
				"AND cm.left_at IS NULL AND c.status = ?",
			conversationID,
			userID,
			conversationStatusOpen,
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

func lockOwnedGroup(
	ctx context.Context,
	tx *gorm.DB,
	actorID uint64,
	conversationID uint64,
) (*conversation, error) {
	var conversationRecord conversation
	if err := tx.WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where(
			"id = ? AND status = ?",
			conversationID,
			conversationStatusOpen,
		).
		First(&conversationRecord).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrConversationNotFound
		}
		return nil, fmt.Errorf("锁定群会话: %w", err)
	}
	if conversationRecord.ConversationType != conversationTypeGroup {
		return nil, ErrNotGroup
	}

	var actor conversationMember
	if err := tx.Where(
		"conversation_id = ? AND user_id = ? AND left_at IS NULL",
		conversationID,
		actorID,
	).First(&actor).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrConversationNotFound
		}
		return nil, fmt.Errorf("校验群管理操作人: %w", err)
	}

	// owner_id 与成员角色必须同时匹配，不能只信任其中一个来源。
	if actor.Role != groupOwnerRole ||
		conversationRecord.OwnerID == nil ||
		*conversationRecord.OwnerID != actorID {
		return nil, ErrPermissionDenied
	}
	return &conversationRecord, nil
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

func hasBlockedPair(
	ctx context.Context,
	db *gorm.DB,
	first uint64,
	second uint64,
) (bool, error) {
	var count int64
	if err := db.WithContext(ctx).
		Table("user_blocks").
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

func (r *Repository) userDisplayName(
	ctx context.Context,
	userID uint64,
) (string, error) {
	var record struct {
		Title string `gorm:"column:title"`
	}
	err := r.db.WithContext(ctx).
		Table("users AS u").
		Select("COALESCE(p.nickname, u.username) AS title").
		Joins("LEFT JOIN user_profiles AS p ON p.user_id = u.id").
		Where("u.id = ?", userID).
		Scan(&record).Error
	if err != nil {
		return "", fmt.Errorf("查询私聊对方资料: %w", err)
	}
	if record.Title == "" {
		return "", ErrUserNotFound
	}
	return record.Title, nil
}

func countFriends(
	ctx context.Context,
	tx *gorm.DB,
	inviterID uint64,
	memberUserIDs []uint64,
) error {
	var friendCount int64
	if err := tx.WithContext(ctx).
		Table("friendships").
		Where(
			"status = ? AND ("+
				"(user_low_id = ? AND user_high_id IN ?) OR "+
				"(user_high_id = ? AND user_low_id IN ?))",
			1,
			inviterID,
			memberUserIDs,
			inviterID,
			memberUserIDs,
		).
		Count(&friendCount).Error; err != nil {
		return fmt.Errorf("校验群成员好友关系: %w", err)
	}
	if friendCount != int64(len(memberUserIDs)) {
		return ErrNotFriend
	}
	return nil
}

func isDuplicateKey(err error) bool {
	var mysqlErr *mysql.MySQLError
	return errors.As(err, &mysqlErr) && mysqlErr.Number == 1062
}

func isForeignKeyError(err error) bool {
	var mysqlErr *mysql.MySQLError
	return errors.As(err, &mysqlErr) && mysqlErr.Number == 1452
}

func nullableString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
