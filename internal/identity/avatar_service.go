package identity

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/PYang176/RelayIM/internal/platform/snowflake"
)

const maxAvatarBytes = 5 << 20

var avatarExtensions = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/webp": ".webp",
	"image/gif":  ".gif",
}

// AvatarStorage 是头像服务依赖的对象存储能力。
type AvatarStorage interface {
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

// UploadAvatarCommand 是上传当前用户头像的参数。
type UploadAvatarCommand struct {
	UserID      uint64
	ContentType string
	Size        int64
	Reader      io.Reader
}

// OpenAvatarResult 是头像图片读取流。
type OpenAvatarResult struct {
	Reader      io.ReadCloser
	ContentType string
	SizeBytes   int64
}

// NewServiceWithAvatarStorage 创建支持头像上传的用户服务。
func NewServiceWithAvatarStorage(
	repo *Repository,
	token TokenIssuer,
	avatars AvatarStorage,
) *Service {
	service := NewService(repo, token)
	service.avatars = avatars
	return service
}

// UploadAvatar 上传图片对象并更新当前用户的稳定头像地址。
func (s *Service) UploadAvatar(
	ctx context.Context,
	command UploadAvatarCommand,
) (UserResult, error) {
	if s.avatars == nil {
		return UserResult{}, ErrAvatarStorageUnavailable
	}
	command.ContentType = strings.TrimSpace(command.ContentType)
	extension, supported := avatarExtensions[command.ContentType]
	if command.UserID == 0 ||
		command.Reader == nil ||
		command.Size <= 0 ||
		command.Size > maxAvatarBytes ||
		!supported {
		return UserResult{}, ErrInvalidAvatar
	}

	_, profile, err := s.repo.FindByID(ctx, command.UserID)
	if err != nil {
		return UserResult{}, err
	}

	now := time.Now().UTC()
	objectKey := fmt.Sprintf(
		"users/%d/avatar/%d%s",
		command.UserID,
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
		return UserResult{}, fmt.Errorf(
			"%w: %v",
			ErrAvatarStorageUnavailable,
			err,
		)
	}

	avatarURL := fmt.Sprintf(
		"/api/v1/users/%d/avatar?v=%d",
		command.UserID,
		now.UnixMilli(),
	)
	if err := s.repo.UpdateAvatar(
		ctx,
		command.UserID,
		&objectKey,
		avatarURL,
		now,
	); err != nil {
		s.cleanupAvatarObject(ctx, objectKey)
		return UserResult{}, err
	}
	if profile.AvatarObjectKey != nil &&
		*profile.AvatarObjectKey != "" {
		s.cleanupAvatarObject(ctx, *profile.AvatarObjectKey)
	}
	return s.Me(ctx, command.UserID)
}

// RemoveAvatar 清除当前用户头像及其对象存储文件。
func (s *Service) RemoveAvatar(
	ctx context.Context,
	userID uint64,
) (UserResult, error) {
	if userID == 0 {
		return UserResult{}, ErrInvalidInput
	}
	_, profile, err := s.repo.FindByID(ctx, userID)
	if err != nil {
		return UserResult{}, err
	}
	if err := s.repo.UpdateAvatar(
		ctx,
		userID,
		nil,
		"",
		time.Now().UTC(),
	); err != nil {
		return UserResult{}, err
	}
	if profile.AvatarObjectKey != nil &&
		*profile.AvatarObjectKey != "" {
		s.cleanupAvatarObject(ctx, *profile.AvatarObjectKey)
	}
	return s.Me(ctx, userID)
}

// OpenAvatar 读取用户公开头像对象。
func (s *Service) OpenAvatar(
	ctx context.Context,
	userID uint64,
) (OpenAvatarResult, error) {
	if s.avatars == nil {
		return OpenAvatarResult{}, ErrAvatarStorageUnavailable
	}
	if userID == 0 {
		return OpenAvatarResult{}, ErrInvalidInput
	}
	_, profile, err := s.repo.FindByID(ctx, userID)
	if err != nil {
		return OpenAvatarResult{}, err
	}
	if profile.AvatarObjectKey == nil ||
		*profile.AvatarObjectKey == "" {
		return OpenAvatarResult{}, ErrAvatarNotFound
	}
	reader, contentType, size, err := s.avatars.OpenObject(
		ctx,
		*profile.AvatarObjectKey,
	)
	if err != nil {
		return OpenAvatarResult{}, fmt.Errorf(
			"%w: %v",
			ErrAvatarStorageUnavailable,
			err,
		)
	}
	return OpenAvatarResult{
		Reader:      reader,
		ContentType: contentType,
		SizeBytes:   size,
	}, nil
}

// cleanupAvatarObject 清理头像对象遗留资源
func (s *Service) cleanupAvatarObject(ctx context.Context, objectKey string) {
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
