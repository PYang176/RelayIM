package conversation

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/PYang176/RelayIM/internal/platform/snowflake"
)

const maxGroupAvatarImageBytes = 5 << 20

var groupAvatarExtensions = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/webp": ".webp",
	"image/gif":  ".gif",
}

// GroupAvatarStorage 是群头像服务依赖的对象存储能力。
type GroupAvatarStorage interface {
	PutObject(
		ctx context.Context,
		objectKey string,
		reader io.Reader,
		size int64,
		contentType string,
	) error
	OpenObject(
		ctx context.Context,
		objectKey string,
	) (io.ReadCloser, string, int64, error)
	RemoveObject(ctx context.Context, objectKey string) error
}

// UploadGroupAvatarCommand 是群主上传群头像的参数。
type UploadGroupAvatarCommand struct {
	// ActorID 是执行上传的群主 Snowflake ID。
	ActorID uint64

	// ConversationID 是目标群会话 Snowflake ID。
	ConversationID uint64

	// ContentType 是服务端识别出的图片 MIME 类型。
	ContentType string

	// Size 是图片字节数。
	Size int64

	// Reader 是图片内容流。
	Reader io.Reader
}

// GroupAvatarResult 是群头像更新后的公开地址。
type GroupAvatarResult struct {
	// AvatarURL 是带版本参数的稳定头像地址。
	AvatarURL string
}

// OpenGroupAvatarResult 是群头像图片读取流。
type OpenGroupAvatarResult struct {
	Reader      io.ReadCloser
	ContentType string
	SizeBytes   int64
}

// UploadGroupAvatar 上传群头像对象并把新地址原子写入会话。
func (s *Service) UploadGroupAvatar(
	ctx context.Context,
	command UploadGroupAvatarCommand,
) (GroupAvatarResult, error) {
	if s.avatars == nil {
		return GroupAvatarResult{}, ErrGroupAvatarStorageUnavailable
	}
	command.ContentType = strings.TrimSpace(command.ContentType)
	extension, supported := groupAvatarExtensions[command.ContentType]
	if command.ActorID == 0 ||
		command.ConversationID == 0 ||
		command.Reader == nil ||
		command.Size <= 0 ||
		command.Size > maxGroupAvatarImageBytes ||
		!supported {
		return GroupAvatarResult{}, ErrInvalidGroupAvatar
	}

	now := time.Now().UTC()
	objectKey := fmt.Sprintf(
		"conversations/%d/avatar/%d%s",
		command.ConversationID,
		uint64(snowflake.Generate()),
		extension,
	)
	if err := s.avatars.PutObject(
		ctx,
		objectKey,
		command.Reader,
		command.Size,
		command.ContentType,
	); err != nil {
		return GroupAvatarResult{}, fmt.Errorf(
			"%w: %v",
			ErrGroupAvatarStorageUnavailable,
			err,
		)
	}

	avatarURL := fmt.Sprintf(
		"/api/v1/conversations/%d/avatar?v=%d",
		command.ConversationID,
		now.UnixMilli(),
	)
	previousObjectKey, err := s.repo.UpdateGroupAvatar(
		ctx,
		command.ActorID,
		command.ConversationID,
		&objectKey,
		avatarURL,
	)
	if err != nil {
		s.cleanupGroupAvatarObject(ctx, objectKey)
		return GroupAvatarResult{}, err
	}
	s.cleanupGroupAvatarObject(ctx, previousObjectKey)
	return GroupAvatarResult{AvatarURL: avatarURL}, nil
}

// RemoveGroupAvatar 清除群头像并删除已有对象。
func (s *Service) RemoveGroupAvatar(
	ctx context.Context,
	actorID uint64,
	conversationID uint64,
) (GroupAvatarResult, error) {
	if actorID == 0 || conversationID == 0 {
		return GroupAvatarResult{}, ErrInvalidInput
	}
	previousObjectKey, err := s.repo.UpdateGroupAvatar(
		ctx,
		actorID,
		conversationID,
		nil,
		"",
	)
	if err != nil {
		return GroupAvatarResult{}, err
	}
	s.cleanupGroupAvatarObject(ctx, previousObjectKey)
	return GroupAvatarResult{}, nil
}

// OpenGroupAvatar 读取公开群头像，供浏览器图片标签直接加载。
func (s *Service) OpenGroupAvatar(
	ctx context.Context,
	conversationID uint64,
) (OpenGroupAvatarResult, error) {
	if s.avatars == nil {
		return OpenGroupAvatarResult{}, ErrGroupAvatarStorageUnavailable
	}
	if conversationID == 0 {
		return OpenGroupAvatarResult{}, ErrInvalidInput
	}
	objectKey, err := s.repo.FindGroupAvatarObjectKey(ctx, conversationID)
	if err != nil {
		return OpenGroupAvatarResult{}, err
	}
	reader, contentType, size, err := s.avatars.OpenObject(ctx, objectKey)
	if err != nil {
		return OpenGroupAvatarResult{}, fmt.Errorf(
			"%w: %v",
			ErrGroupAvatarStorageUnavailable,
			err,
		)
	}
	return OpenGroupAvatarResult{
		Reader:      reader,
		ContentType: contentType,
		SizeBytes:   size,
	}, nil
}

// cleanupGroupAvatarObject 清理不再引用的群头像对象。
func (s *Service) cleanupGroupAvatarObject(
	ctx context.Context,
	objectKey string,
) {
	if s.avatars == nil || objectKey == "" {
		return
	}
	cleanupCtx, cancel := context.WithTimeout(
		context.WithoutCancel(ctx),
		5*time.Second,
	)
	defer cancel()
	_ = s.avatars.RemoveObject(cleanupCtx, objectKey)
}
