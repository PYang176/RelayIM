package conversation

import (
	"context"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	maxGroupNameRunes   = 64
	maxGroupMemberCount = 500
	maxGroupAvatarBytes = 512
)

// Service 提供会话创建与列表查询。
type Service struct {
	repo *Repository
}

// NewService 创建会话服务。
func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

// CreateDirectCommand 是创建私聊会话的参数。
type CreateDirectCommand struct {
	// UserID 是当前登录用户 Snowflake ID。
	UserID uint64

	// PeerUserID 是聊天对方 Snowflake ID。
	PeerUserID uint64
}

// CreateGroupCommand 是创建群会话的参数。
type CreateGroupCommand struct {
	// OwnerID 是群主 Snowflake ID。
	OwnerID uint64

	// Name 是群名称。
	Name string

	// AvatarURL 是可选群头像地址。
	AvatarURL string

	// MemberUserIDs 是创建群时一并加入的好友 Snowflake ID。
	MemberUserIDs []uint64
}

// InviteMembersCommand 是群主或管理员继续邀请成员的参数。
type InviteMembersCommand struct {
	// ActorID 是执行邀请的用户 Snowflake ID。
	ActorID uint64

	// ConversationID 是群会话 Snowflake ID。
	ConversationID uint64

	// MemberUserIDs 是待邀请好友 Snowflake ID。
	MemberUserIDs []uint64
}

// RemoveMemberCommand 是群主移除成员或普通成员退群的参数。
type RemoveMemberCommand struct {
	// ActorID 是执行操作的用户 Snowflake ID。
	ActorID uint64

	// ConversationID 是群会话 Snowflake ID。
	ConversationID uint64

	// TargetUserID 是被移除或主动退群的用户 Snowflake ID。
	TargetUserID uint64
}

// SetMemberRoleCommand 是群主设置或取消群管理员的参数。
type SetMemberRoleCommand struct {
	// ActorID 是执行操作的群主 Snowflake ID。
	ActorID uint64

	// ConversationID 是群会话 Snowflake ID。
	ConversationID uint64

	// TargetUserID 是目标成员 Snowflake ID。
	TargetUserID uint64

	// Role 是目标角色，1 为普通成员、2 为管理员。
	Role int
}

// TransferOwnerCommand 是群主转让群聊所有权参数。
type TransferOwnerCommand struct {
	// ActorID 是当前群主 Snowflake ID。
	ActorID uint64

	// ConversationID 是群会话 Snowflake ID。
	ConversationID uint64

	// TargetUserID 是接管群主身份的有效成员 Snowflake ID。
	TargetUserID uint64
}

// DissolveGroupCommand 是群主解散群的参数。
type DissolveGroupCommand struct {
	// ActorID 是执行解散操作的群主 Snowflake ID。
	ActorID uint64

	// ConversationID 是群会话 Snowflake ID。
	ConversationID uint64
}

// UpdateGroupProfileCommand 是群主修改群名称和头像的参数。
type UpdateGroupProfileCommand struct {
	// ActorID 是执行修改的群主 Snowflake ID。
	ActorID uint64

	// ConversationID 是群会话 Snowflake ID。
	ConversationID uint64

	// Name 是新的群名称。
	Name string

	// AvatarURL 是可选群头像地址，空字符串表示清除头像。
	AvatarURL string
}

// GroupProfileResult 是群资料修改后的公开摘要。
type GroupProfileResult struct {
	// ConversationID 是群会话 Snowflake ID。
	ConversationID uint64

	// Name 是更新后的群名称。
	Name string

	// AvatarURL 是更新后的群头像地址。
	AvatarURL string
}

// Result 是接口层可安全使用的会话信息。
type Result struct {
	// ID 是会话 Snowflake ID。
	ID uint64

	// ConversationType 表示私聊或群聊。
	ConversationType int

	// Name 是群名称，私聊时为空。
	Name string

	// Title 是可直接展示的会话标题，私聊使用对方昵称。
	Title string

	// AvatarURL 是群头像。
	AvatarURL string

	// LastSeq 是会话最后消息序号。
	LastSeq uint64

	// LastReadSeq 是当前用户最后已读序号。
	LastReadSeq uint64

	// UnreadCount 是当前用户未读消息数。
	UnreadCount uint64

	// MemberCount 是会话当前有效成员数量。
	MemberCount uint64
}

// MemberResult 是接口层可安全展示的会话成员。
type MemberResult struct {
	// UserID 是成员 Snowflake ID。
	UserID uint64

	// Username 是成员登录名。
	Username string

	// Nickname 是成员昵称。
	Nickname string

	// AvatarURL 是成员头像地址。
	AvatarURL string

	// Role 表示成员角色，1 成员、2 管理员、3 群主。
	Role int

	// JoinedAt 是成员加入会话的 UTC 时间。
	JoinedAt time.Time
}

// CreateDirect 创建或获取当前用户与另一用户的唯一私聊。
func (s *Service) CreateDirect(
	ctx context.Context,
	command CreateDirectCommand,
) (Result, bool, error) {
	if command.UserID == 0 || command.PeerUserID == 0 {
		return Result{}, false, ErrInvalidInput
	}
	if command.UserID == command.PeerUserID {
		return Result{}, false, ErrSameUser
	}

	record, created, err := s.repo.CreateDirect(
		ctx,
		command.UserID,
		command.PeerUserID,
	)
	if err != nil {
		return Result{}, false, err
	}
	return Result{
		ID:               record.ID,
		ConversationType: record.ConversationType,
		Name:             stringValue(record.Name),
		Title:            record.Title,
		AvatarURL:        stringValue(record.AvatarURL),
		LastSeq:          record.LastSeq,
		MemberCount:      2,
	}, created, nil
}

// CreateGroup 创建群会话并自动加入群主。
func (s *Service) CreateGroup(
	ctx context.Context,
	command CreateGroupCommand,
) (Result, error) {
	command.Name = strings.TrimSpace(command.Name)
	command.AvatarURL = strings.TrimSpace(command.AvatarURL)
	if command.OwnerID == 0 ||
		command.Name == "" ||
		utf8.RuneCountInString(command.Name) > maxGroupNameRunes {
		return Result{}, ErrInvalidInput
	}

	memberUserIDs, err := normalizeGroupMemberIDs(
		command.OwnerID,
		command.MemberUserIDs,
	)
	if err != nil {
		return Result{}, err
	}

	var avatarURL *string
	if command.AvatarURL != "" {
		avatarURL = &command.AvatarURL
	}

	record, err := s.repo.CreateGroup(
		ctx,
		command.OwnerID,
		command.Name,
		avatarURL,
		memberUserIDs,
	)
	if err != nil {
		return Result{}, err
	}
	return Result{
		ID:               record.ID,
		ConversationType: record.ConversationType,
		Name:             stringValue(record.Name),
		Title:            stringValue(record.Name),
		AvatarURL:        stringValue(record.AvatarURL),
		LastSeq:          record.LastSeq,
		MemberCount:      uint64(len(memberUserIDs) + 1),
	}, nil
}

// InviteMembers 邀请群主或管理员的更多好友加入群聊。
func (s *Service) InviteMembers(
	ctx context.Context,
	command InviteMembersCommand,
) (uint64, error) {
	if command.ActorID == 0 || command.ConversationID == 0 {
		return 0, ErrInvalidInput
	}
	memberUserIDs, err := normalizeGroupMemberIDs(
		command.ActorID,
		command.MemberUserIDs,
	)
	if err != nil {
		return 0, err
	}
	return s.repo.InviteMembers(
		ctx,
		command.ActorID,
		command.ConversationID,
		memberUserIDs,
	)
}

// RemoveMember 处理群主移除成员或普通成员主动退群。
func (s *Service) RemoveMember(
	ctx context.Context,
	command RemoveMemberCommand,
) error {
	if command.ActorID == 0 ||
		command.ConversationID == 0 ||
		command.TargetUserID == 0 {
		return ErrInvalidInput
	}
	return s.repo.RemoveMember(
		ctx,
		command.ActorID,
		command.ConversationID,
		command.TargetUserID,
	)
}

// SetMemberRole 由群主设置管理员或取消管理员。
func (s *Service) SetMemberRole(
	ctx context.Context,
	command SetMemberRoleCommand,
) error {
	if command.ActorID == 0 ||
		command.ConversationID == 0 ||
		command.TargetUserID == 0 ||
		(command.Role != groupMemberRole &&
			command.Role != groupAdminRole) {
		return ErrInvalidInput
	}
	return s.repo.SetMemberRole(
		ctx,
		command.ActorID,
		command.ConversationID,
		command.TargetUserID,
		command.Role,
	)
}

// TransferOwner 将群主身份转让给另一位有效成员。
func (s *Service) TransferOwner(
	ctx context.Context,
	command TransferOwnerCommand,
) error {
	if command.ActorID == 0 ||
		command.ConversationID == 0 ||
		command.TargetUserID == 0 ||
		command.ActorID == command.TargetUserID {
		return ErrInvalidInput
	}
	return s.repo.TransferOwner(
		ctx,
		command.ActorID,
		command.ConversationID,
		command.TargetUserID,
	)
}

// DissolveGroup 由群主关闭群会话，历史成员和消息保留用于审计。
func (s *Service) DissolveGroup(
	ctx context.Context,
	command DissolveGroupCommand,
) error {
	if command.ActorID == 0 || command.ConversationID == 0 {
		return ErrInvalidInput
	}
	return s.repo.DissolveGroup(
		ctx,
		command.ActorID,
		command.ConversationID,
	)
}

// UpdateGroupProfile 校验群主权限和公开字段后更新群名称与头像。
func (s *Service) UpdateGroupProfile(
	ctx context.Context,
	command UpdateGroupProfileCommand,
) (GroupProfileResult, error) {
	command.Name = strings.TrimSpace(command.Name)
	command.AvatarURL = strings.TrimSpace(command.AvatarURL)
	if command.ActorID == 0 ||
		command.ConversationID == 0 ||
		command.Name == "" ||
		utf8.RuneCountInString(command.Name) > maxGroupNameRunes ||
		!validGroupAvatarURL(command.AvatarURL) {
		return GroupProfileResult{}, ErrInvalidInput
	}
	if err := s.repo.UpdateGroupProfile(
		ctx,
		command.ActorID,
		command.ConversationID,
		command.Name,
		command.AvatarURL,
	); err != nil {
		return GroupProfileResult{}, err
	}
	return GroupProfileResult{
		ConversationID: command.ConversationID,
		Name:           command.Name,
		AvatarURL:      command.AvatarURL,
	}, nil
}

// ListMembers 返回当前用户有权查看的会话有效成员。
func (s *Service) ListMembers(
	ctx context.Context,
	userID uint64,
	conversationID uint64,
) ([]MemberResult, error) {
	if userID == 0 || conversationID == 0 {
		return nil, ErrInvalidInput
	}
	records, err := s.repo.ListMembers(ctx, userID, conversationID)
	if err != nil {
		return nil, err
	}
	results := make([]MemberResult, 0, len(records))
	for _, record := range records {
		results = append(results, MemberResult{
			UserID:    record.UserID,
			Username:  record.Username,
			Nickname:  record.Nickname,
			AvatarURL: record.AvatarURL,
			Role:      record.Role,
			JoinedAt:  record.JoinedAt,
		})
	}
	return results, nil
}

// List 返回当前用户的会话列表。
func (s *Service) List(ctx context.Context, userID uint64) ([]Result, error) {
	if userID == 0 {
		return nil, ErrInvalidInput
	}

	records, err := s.repo.ListForUser(ctx, userID)
	if err != nil {
		return nil, err
	}

	results := make([]Result, 0, len(records))
	for _, record := range records {
		unread := uint64(0)
		if record.LastSeq > record.LastReadSeq {
			unread = record.LastSeq - record.LastReadSeq
		}
		results = append(results, Result{
			ID:               record.ID,
			ConversationType: record.ConversationType,
			Name:             stringValue(record.Name),
			Title:            record.Title,
			AvatarURL:        stringValue(record.AvatarURL),
			LastSeq:          record.LastSeq,
			LastReadSeq:      record.LastReadSeq,
			UnreadCount:      unread,
			MemberCount:      record.MemberCount,
		})
	}
	return results, nil
}

func normalizeGroupMemberIDs(
	ownerID uint64,
	userIDs []uint64,
) ([]uint64, error) {
	seen := make(map[uint64]struct{}, len(userIDs))
	normalized := make([]uint64, 0, len(userIDs))
	for _, userID := range userIDs {
		if userID == 0 || userID == ownerID {
			return nil, ErrInvalidInput
		}
		if _, exists := seen[userID]; exists {
			continue
		}
		seen[userID] = struct{}{}
		normalized = append(normalized, userID)
	}
	if len(normalized) == 0 || len(normalized)+1 > maxGroupMemberCount {
		return nil, ErrInvalidInput
	}
	return normalized, nil
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func validGroupAvatarURL(value string) bool {
	if value == "" {
		return true
	}
	if len(value) > maxGroupAvatarBytes {
		return false
	}
	parsed, err := url.ParseRequestURI(value)
	if err != nil || parsed.Host == "" {
		return false
	}
	return parsed.Scheme == "http" || parsed.Scheme == "https"
}
