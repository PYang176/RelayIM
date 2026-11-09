package conversation

import (
	"context"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	maxGroupNameRunes          = 64
	maxGroupMemberCount        = 500
	maxGroupAvatarBytes        = 512
	maxInvitationRemarkRunes   = 200
	maxInvitationResponseRunes = 200
	maxConversationSearchRunes = 64
	defaultPageSize            = 20
	maxPageSize                = 100
	invitationTTL              = 7 * 24 * time.Hour
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

	// InviteRemark 是邀请人填写的入群理由。
	InviteRemark string
}

// InviteMembersCommand 是群主或管理员继续邀请成员的参数。
type InviteMembersCommand struct {
	// ActorID 是执行邀请的用户 Snowflake ID。
	ActorID uint64

	// ConversationID 是群会话 Snowflake ID。
	ConversationID uint64

	// MemberUserIDs 是待邀请好友 Snowflake ID。
	MemberUserIDs []uint64

	// InviteRemark 是邀请人填写的入群理由。
	InviteRemark string
}

// InvitationPageQuery 是群邀请列表的游标分页参数。
type InvitationPageQuery struct {
	// Limit 是本页最大记录数。
	Limit int

	// BeforeID 是上一页最后一条邀请的 Snowflake ID。
	BeforeID uint64
}

// ConversationPageQuery 是会话列表的复合游标分页参数。
type ConversationPageQuery struct {
	// Limit 是本页最大记录数。
	Limit int

	// Search 是按会话展示标题匹配的关键字，空字符串表示不过滤。
	Search string

	// BeforePinned 是上一页最后一条会话是否位于置顶分组。
	BeforePinned bool

	// BeforeUpdatedAt 是上一页最后一条会话的 UTC 更新时间。
	BeforeUpdatedAt time.Time

	// BeforeID 是上一页最后一条会话的 Snowflake ID。
	BeforeID uint64
}

// RespondInvitationCommand 是接受或拒绝群邀请的参数。
type RespondInvitationCommand struct {
	// UserID 是被邀请人 Snowflake ID。
	UserID uint64

	// InvitationID 是群邀请 Snowflake ID。
	InvitationID uint64

	// ResponseRemark 是可选审批回复。
	ResponseRemark string
}

// CancelInvitationCommand 是群管理员取消邀请的参数。
type CancelInvitationCommand struct {
	// ActorID 是执行取消的群主或管理员 Snowflake ID。
	ActorID uint64

	// ConversationID 是群会话 Snowflake ID。
	ConversationID uint64

	// InvitationID 是待取消邀请 Snowflake ID。
	InvitationID uint64
}

// ReceivedInvitationResult 是当前用户收到的一条群邀请。
type ReceivedInvitationResult struct {
	// InvitationID 是邀请 Snowflake ID。
	InvitationID uint64

	// ConversationID 是目标群会话 Snowflake ID。
	ConversationID uint64

	// ConversationName 是群名称。
	ConversationName string

	// AvatarURL 是群头像。
	AvatarURL string

	// InviterID 是邀请人 Snowflake ID。
	InviterID uint64

	// InviterName 是邀请人的公开昵称。
	InviterName string

	// InviteRemark 是邀请人填写的入群理由。
	InviteRemark string

	// CreatedAt 是邀请创建时间。
	CreatedAt time.Time

	// ExpiresAt 是邀请失效时间。
	ExpiresAt time.Time
}

// ReceivedInvitationPage 是当前用户收到的群邀请一页。
type ReceivedInvitationPage struct {
	Invitations []ReceivedInvitationResult
	NextCursor  uint64
	HasMore     bool
}

// GroupInvitationResult 是群管理员看到的邀请记录。
type GroupInvitationResult struct {
	InvitationID   uint64
	Invitee        MemberResult
	InviterID      uint64
	InviteRemark   string
	Status         int
	ResponseRemark string
	CreatedAt      time.Time
	ExpiresAt      time.Time
	RespondedAt    *time.Time
}

// GroupInvitationPage 是群待处理邀请的一页。
type GroupInvitationPage struct {
	Invitations []GroupInvitationResult
	NextCursor  uint64
	HasMore     bool
}

// InvitationAcceptanceResult 是接受邀请后的群状态。
type InvitationAcceptanceResult struct {
	InvitationID   uint64
	ConversationID uint64
	MemberCount    uint64
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

	// PeerUserID 是私聊对方 Snowflake ID，群聊为零。
	PeerUserID uint64

	// AvatarURL 是群头像。
	AvatarURL string

	// LastSeq 是会话最后消息序号。
	LastSeq uint64

	// LastReadSeq 是当前用户最后已读序号。
	LastReadSeq uint64

	// Pinned 表示当前用户是否将会话置顶。
	Pinned bool

	// UnreadCount 是当前用户未读消息数。
	UnreadCount uint64

	// MemberCount 是会话当前有效成员数量。
	MemberCount uint64

	// UpdatedAt 是会话最近活动的 UTC 时间。
	UpdatedAt time.Time
}

// ConversationPage 是会话列表的一页。
type ConversationPage struct {
	// Conversations 是本页按置顶状态和最近活动时间排列的会话。
	Conversations []Result

	// NextPinned 是下一页游标中最后一条会话的置顶状态。
	NextPinned bool

	// NextUpdatedAt 是下一页游标中的最后活动时间。
	NextUpdatedAt time.Time

	// NextID 是下一页游标中的会话 Snowflake ID。
	NextID uint64

	// HasMore 表示当前游标之后是否仍有会话。
	HasMore bool
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
		PeerUserID:       command.PeerUserID,
		AvatarURL:        stringValue(record.AvatarURL),
		LastSeq:          record.LastSeq,
		MemberCount:      2,
		UpdatedAt:        record.UpdatedAt,
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
	inviteRemark, err := normalizeInvitationRemark(command.InviteRemark)
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
		inviteRemark,
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
		MemberCount:      1,
		UpdatedAt:        record.UpdatedAt,
	}, nil
}

// InviteMembers 由群主或管理员向更多好友发送入群邀请。
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
	inviteRemark, err := normalizeInvitationRemark(command.InviteRemark)
	if err != nil {
		return 0, err
	}
	return s.repo.InviteMembers(
		ctx,
		command.ActorID,
		command.ConversationID,
		memberUserIDs,
		inviteRemark,
	)
}

// ListReceivedInvitations 返回当前用户待处理的群邀请。
func (s *Service) ListReceivedInvitations(
	ctx context.Context,
	userID uint64,
	page InvitationPageQuery,
) (ReceivedInvitationPage, error) {
	if userID == 0 || !validInvitationPage(page) {
		return ReceivedInvitationPage{}, ErrInvalidInput
	}
	return s.repo.listReceivedInvitations(
		ctx,
		userID,
		page.Limit,
		page.BeforeID,
	)
}

// AcceptInvitation 接受群邀请并在事务中加入群。
func (s *Service) AcceptInvitation(
	ctx context.Context,
	command RespondInvitationCommand,
) (InvitationAcceptanceResult, error) {
	if command.UserID == 0 || command.InvitationID == 0 {
		return InvitationAcceptanceResult{}, ErrInvalidInput
	}
	responseRemark, err := normalizeInvitationResponse(
		command.ResponseRemark,
	)
	if err != nil {
		return InvitationAcceptanceResult{}, err
	}
	record, memberCount, err := s.repo.respondToInvitation(
		ctx,
		command.UserID,
		command.InvitationID,
		true,
		responseRemark,
	)
	if err != nil {
		return InvitationAcceptanceResult{}, err
	}
	return InvitationAcceptanceResult{
		InvitationID:   record.ID,
		ConversationID: record.ConversationID,
		MemberCount:    memberCount,
	}, nil
}

// RejectInvitation 拒绝群邀请。
func (s *Service) RejectInvitation(
	ctx context.Context,
	command RespondInvitationCommand,
) error {
	if command.UserID == 0 || command.InvitationID == 0 {
		return ErrInvalidInput
	}
	responseRemark, err := normalizeInvitationResponse(
		command.ResponseRemark,
	)
	if err != nil {
		return err
	}
	_, _, err = s.repo.respondToInvitation(
		ctx,
		command.UserID,
		command.InvitationID,
		false,
		responseRemark,
	)
	return err
}

// CancelInvitation 由群主或管理员取消待处理邀请。
func (s *Service) CancelInvitation(
	ctx context.Context,
	command CancelInvitationCommand,
) error {
	if command.ActorID == 0 ||
		command.ConversationID == 0 ||
		command.InvitationID == 0 {
		return ErrInvalidInput
	}
	return s.repo.cancelInvitation(
		ctx,
		command.ActorID,
		command.ConversationID,
		command.InvitationID,
	)
}

// ListGroupInvitations 返回群主或管理员可见的待处理邀请。
func (s *Service) ListGroupInvitations(
	ctx context.Context,
	actorID uint64,
	conversationID uint64,
	page InvitationPageQuery,
) (GroupInvitationPage, error) {
	if actorID == 0 ||
		conversationID == 0 ||
		!validInvitationPage(page) {
		return GroupInvitationPage{}, ErrInvalidInput
	}
	return s.repo.listGroupInvitations(
		ctx,
		actorID,
		conversationID,
		page.Limit,
		page.BeforeID,
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
func (s *Service) List(
	ctx context.Context,
	userID uint64,
	page ConversationPageQuery,
) (ConversationPage, error) {
	page.Search = strings.TrimSpace(page.Search)
	if userID == 0 ||
		page.Limit <= 0 ||
		page.Limit > maxPageSize ||
		utf8.RuneCountInString(page.Search) > maxConversationSearchRunes {
		return ConversationPage{}, ErrInvalidInput
	}

	records, hasMore, err := s.repo.ListForUser(ctx, userID, page)
	if err != nil {
		return ConversationPage{}, err
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
			PeerUserID:       uint64Value(record.PeerUserID),
			AvatarURL:        stringValue(record.AvatarURL),
			LastSeq:          record.LastSeq,
			LastReadSeq:      record.LastReadSeq,
			Pinned:           record.Pinned,
			UnreadCount:      unread,
			MemberCount:      record.MemberCount,
			UpdatedAt:        record.UpdatedAt,
		})
	}
	resultPage := ConversationPage{
		Conversations: results,
		HasMore:       hasMore,
	}
	if hasMore && len(results) > 0 {
		last := results[len(results)-1]
		resultPage.NextPinned = last.Pinned
		resultPage.NextUpdatedAt = last.UpdatedAt
		resultPage.NextID = last.ID
	}
	return resultPage, nil
}

// Get 返回当前用户可访问的单个会话。
func (s *Service) Get(
	ctx context.Context,
	userID uint64,
	conversationID uint64,
) (Result, error) {
	if userID == 0 || conversationID == 0 {
		return Result{}, ErrInvalidInput
	}
	record, err := s.repo.GetForUser(ctx, userID, conversationID)
	if err != nil {
		return Result{}, err
	}
	unread := uint64(0)
	if record.LastSeq > record.LastReadSeq {
		unread = record.LastSeq - record.LastReadSeq
	}
	return Result{
		ID:               record.ID,
		ConversationType: record.ConversationType,
		Name:             stringValue(record.Name),
		Title:            record.Title,
		PeerUserID:       uint64Value(record.PeerUserID),
		AvatarURL:        stringValue(record.AvatarURL),
		LastSeq:          record.LastSeq,
		LastReadSeq:      record.LastReadSeq,
		Pinned:           record.Pinned,
		UnreadCount:      unread,
		MemberCount:      record.MemberCount,
		UpdatedAt:        record.UpdatedAt,
	}, nil
}

// SetPinned 修改当前用户自己的会话置顶状态。
func (s *Service) SetPinned(
	ctx context.Context,
	userID uint64,
	conversationID uint64,
	pinned bool,
) error {
	if userID == 0 || conversationID == 0 {
		return ErrInvalidInput
	}
	return s.repo.SetPinned(ctx, userID, conversationID, pinned)
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

func uint64Value(value *uint64) uint64 {
	if value == nil {
		return 0
	}
	return *value
}

func validInvitationPage(page InvitationPageQuery) bool {
	return page.Limit > 0 && page.Limit <= maxPageSize
}

func normalizeInvitationRemark(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", ErrInvitationRemarkRequired
	}
	if utf8.RuneCountInString(value) > maxInvitationRemarkRunes {
		return "", ErrInvalidInput
	}
	return value, nil
}

func normalizeInvitationResponse(value string) (string, error) {
	value = strings.TrimSpace(value)
	if utf8.RuneCountInString(value) > maxInvitationResponseRunes {
		return "", ErrInvalidInput
	}
	return value, nil
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
