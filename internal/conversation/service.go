package conversation

import (
	"context"
	"strings"
	"unicode/utf8"
)

const maxGroupNameRunes = 64

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
}

// Result 是接口层可安全使用的会话信息。
type Result struct {
	// ID 是会话 Snowflake ID。
	ID uint64

	// ConversationType 表示私聊或群聊。
	ConversationType int

	// Name 是群名称，私聊时为空。
	Name string

	// AvatarURL 是群头像。
	AvatarURL string

	// LastSeq 是会话最后消息序号。
	LastSeq uint64

	// LastReadSeq 是当前用户最后已读序号。
	LastReadSeq uint64

	// UnreadCount 是当前用户未读消息数。
	UnreadCount uint64
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
		AvatarURL:        stringValue(record.AvatarURL),
		LastSeq:          record.LastSeq,
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

	var avatarURL *string
	if command.AvatarURL != "" {
		avatarURL = &command.AvatarURL
	}

	record, err := s.repo.CreateGroup(
		ctx,
		command.OwnerID,
		command.Name,
		avatarURL,
	)
	if err != nil {
		return Result{}, err
	}
	return Result{
		ID:               record.ID,
		ConversationType: record.ConversationType,
		Name:             stringValue(record.Name),
		AvatarURL:        stringValue(record.AvatarURL),
		LastSeq:          record.LastSeq,
	}, nil
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
			AvatarURL:        stringValue(record.AvatarURL),
			LastSeq:          record.LastSeq,
			LastReadSeq:      record.LastReadSeq,
			UnreadCount:      unread,
		})
	}
	return results, nil
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
