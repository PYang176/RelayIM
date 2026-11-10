package friendship

import (
	"context"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	maxSearchQueryRunes   = 50
	maxRequestRemarkRunes = 200
	maxFriendRemarkRunes  = 64
	defaultPageSize       = 20
	maxPageSize           = 100
)

// Service 提供好友搜索、申请和关系管理。
type Service struct {
	repo *Repository
}

// NewService 创建好友服务。
func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

// UserResult 是可展示的公开用户资料。
type UserResult struct {
	ID        uint64
	Username  string
	Nickname  string
	AvatarURL string
}

// FriendResult 是好友列表项。
type FriendResult struct {
	FriendshipID uint64
	User         UserResult
	Remark       string
	BlockedByMe  bool
}

// BlockedUserResult 是当前用户拉黑列表项。
type BlockedUserResult struct {
	User      UserResult
	CreatedAt time.Time
}

// BlockedUserPage 是拉黑列表的一页。
type BlockedUserPage struct {
	BlockedUsers []BlockedUserResult
	NextCursor   uint64
	HasMore      bool
}

// RequestResult 是好友申请列表项。
type RequestResult struct {
	ID        uint64
	User      UserResult
	Status    int
	Remark    string
	CreatedAt time.Time
}

// PageQuery 是好友和申请列表的游标分页参数。
type PageQuery struct {
	Limit    int
	BeforeID uint64
}

// FriendPage 是好友列表的一页。
type FriendPage struct {
	Friends    []FriendResult
	NextCursor uint64
	HasMore    bool
}

// RequestPage 是好友申请列表的一页。
type RequestPage struct {
	Requests   []RequestResult
	NextCursor uint64
	HasMore    bool
}

// SearchUsers 按用户名或昵称搜索用户。
func (s *Service) SearchUsers(
	ctx context.Context,
	userID uint64,
	query string,
) ([]UserResult, error) {
	query = strings.TrimSpace(query)
	if userID == 0 ||
		query == "" ||
		utf8.RuneCountInString(query) > maxSearchQueryRunes {
		return nil, ErrInvalidInput
	}
	return s.repo.searchUsers(ctx, userID, query)
}

// SendRequest 创建好友申请并处理重复申请、拉黑和唯一约束
func (s *Service) SendRequest(
	ctx context.Context,
	userID uint64,
	targetUserID uint64,
	remark string,
) error {
	remark = strings.TrimSpace(remark)
	if userID == 0 || targetUserID == 0 || userID == targetUserID {
		return ErrInvalidInput
	}
	if remark == "" {
		return ErrRequestRemarkRequired
	}
	if utf8.RuneCountInString(remark) > maxRequestRemarkRunes {
		return ErrInvalidInput
	}
	return s.repo.sendRequest(ctx, userID, targetUserID, remark)
}

// AcceptRequest 同意好友申请并激活好友关系。
func (s *Service) AcceptRequest(
	ctx context.Context,
	userID uint64,
	requestID uint64,
) error {
	if userID == 0 || requestID == 0 {
		return ErrInvalidInput
	}
	return s.repo.respondToRequest(ctx, userID, requestID, true)
}

// RejectRequest 拒绝好友申请。
func (s *Service) RejectRequest(
	ctx context.Context,
	userID uint64,
	requestID uint64,
) error {
	if userID == 0 || requestID == 0 {
		return ErrInvalidInput
	}
	return s.repo.respondToRequest(ctx, userID, requestID, false)
}

// ListFriends 查询当前用户的好友关系并应用游标分页
func (s *Service) ListFriends(
	ctx context.Context,
	userID uint64,
	page PageQuery,
) (FriendPage, error) {
	if userID == 0 || !validPageQuery(page) {
		return FriendPage{}, ErrInvalidInput
	}
	return s.repo.listFriends(ctx, userID, page.Limit, page.BeforeID)
}

// ListBlockedUsers 返回当前用户主动拉黑的用户。
func (s *Service) ListBlockedUsers(
	ctx context.Context,
	userID uint64,
	page PageQuery,
) (BlockedUserPage, error) {
	if userID == 0 || !validPageQuery(page) {
		return BlockedUserPage{}, ErrInvalidInput
	}
	return s.repo.listBlockedUsers(ctx, userID, page.Limit, page.BeforeID)
}

// BlockUser 创建当前用户到目标用户的方向性拉黑关系。
func (s *Service) BlockUser(
	ctx context.Context,
	userID uint64,
	targetUserID uint64,
) error {
	if userID == 0 || targetUserID == 0 || userID == targetUserID {
		return ErrInvalidInput
	}
	return s.repo.blockUser(ctx, userID, targetUserID)
}

// UnblockUser 删除当前用户到目标用户的拉黑关系。
func (s *Service) UnblockUser(
	ctx context.Context,
	userID uint64,
	targetUserID uint64,
) error {
	if userID == 0 || targetUserID == 0 || userID == targetUserID {
		return ErrInvalidInput
	}
	return s.repo.unblockUser(ctx, userID, targetUserID)
}

// ListRequests 查询好友申请列表并应用游标分页
func (s *Service) ListRequests(
	ctx context.Context,
	userID uint64,
	received bool,
	page PageQuery,
) (RequestPage, error) {
	if userID == 0 || !validPageQuery(page) {
		return RequestPage{}, ErrInvalidInput
	}
	return s.repo.listRequests(ctx, userID, received, page.Limit, page.BeforeID)
}

// validPageQuery 校验好友列表分页大小和游标参数
func validPageQuery(page PageQuery) bool {
	return page.Limit > 0 && page.Limit <= maxPageSize
}

// SetRemark 设置当前用户独立的好友备注；空备注表示删除。
func (s *Service) SetRemark(
	ctx context.Context,
	userID uint64,
	friendUserID uint64,
	remark string,
) error {
	remark = strings.TrimSpace(remark)
	if userID == 0 || friendUserID == 0 || userID == friendUserID {
		return ErrInvalidInput
	}
	if utf8.RuneCountInString(remark) > maxFriendRemarkRunes {
		return ErrInvalidInput
	}
	return s.repo.setRemark(ctx, userID, friendUserID, remark)
}

// DeleteFriend 软删除当前用户与目标用户的好友关系。
func (s *Service) DeleteFriend(
	ctx context.Context,
	userID uint64,
	friendUserID uint64,
) error {
	if userID == 0 || friendUserID == 0 || userID == friendUserID {
		return ErrInvalidInput
	}
	return s.repo.deleteFriend(ctx, userID, friendUserID)
}
