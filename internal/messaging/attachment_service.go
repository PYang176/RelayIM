package messaging

import (
	"context"
	"fmt"
	"io"
	"path"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/PYang176/RelayIM/internal/platform/snowflake"
)

const (
	maxAttachmentBytes = 25 << 20
	maxFilenameRunes   = 180
)

// AttachmentStorage 是附件服务依赖的对象存储能力。
type AttachmentStorage interface {
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
	StatObject(
		ctx context.Context,
		objectKey string,
	) (string, int64, error)
	RemoveObject(ctx context.Context, objectKey string) error
	PresignedGetURL(
		ctx context.Context,
		objectKey string,
		expiry time.Duration,
	) (string, error)
}

// UploadAttachmentCommand 是上传附件的业务参数。
type UploadAttachmentCommand struct {
	OwnerID     uint64
	Filename    string
	ContentType string
	Size        int64
	Reader      io.Reader
}

// AttachmentResult 是可返回给客户端的附件信息。
type AttachmentResult struct {
	ID          uint64
	MessageType int
	MIMEType    string
	SizeBytes   int64
	URL         string
	Filename    string
}

// OpenAttachmentResult 是附件下载流。
type OpenAttachmentResult struct {
	Reader    io.ReadCloser
	MIMEType  string
	SizeBytes int64
	Filename  string
}

// NewServiceWithStorage 创建同时支持消息与附件的服务。
func NewServiceWithStorage(
	repo *Repository,
	storage AttachmentStorage,
) *Service {
	return &Service{
		repo:    repo,
		storage: storage,
	}
}

// UploadAttachment 保存元数据并上传对象，成功后确认附件可用。
func (s *Service) UploadAttachment(
	ctx context.Context,
	command UploadAttachmentCommand,
) (AttachmentResult, error) {
	if s.storage == nil {
		return AttachmentResult{}, ErrAttachmentStorageUnavailable
	}

	command.Filename = safeFilename(command.Filename)
	command.ContentType = strings.TrimSpace(command.ContentType)
	if command.OwnerID == 0 ||
		command.Reader == nil ||
		command.Size <= 0 ||
		command.Size > maxAttachmentBytes ||
		command.Filename == "" ||
		command.ContentType == "" {
		return AttachmentResult{}, ErrInvalidAttachment
	}

	now := time.Now().UTC()
	attachmentID := uint64(snowflake.Generate())
	extension := strings.ToLower(path.Ext(command.Filename))
	objectKey := fmt.Sprintf(
		"users/%d/attachments/%d%s",
		command.OwnerID,
		attachmentID,
		extension,
	)
	record := &attachment{
		ID:           attachmentID,
		OwnerID:      command.OwnerID,
		ObjectKey:    objectKey,
		OriginalName: command.Filename,
		MIMEType:     command.ContentType,
		SizeBytes:    command.Size,
		Status:       attachmentStatusPending,
		CreatedAt:    now,
	}
	if err := s.repo.createAttachment(ctx, record); err != nil {
		return AttachmentResult{}, err
	}

	if err := s.storage.PutObject(
		ctx,
		objectKey,
		command.Reader,
		command.Size,
		command.ContentType,
	); err != nil {
		s.cleanupStoredAttachment(ctx, attachmentID, objectKey)
		return AttachmentResult{}, fmt.Errorf(
			"%w: %v",
			ErrAttachmentStorageUnavailable,
			err,
		)
	}
	if err := s.repo.finalizeAttachment(ctx, attachmentID, now); err != nil {
		s.cleanupStoredAttachment(ctx, attachmentID, objectKey)
		return AttachmentResult{}, err
	}

	downloadURL, err := s.storage.PresignedGetURL(ctx, objectKey, 15*time.Minute)
	if err != nil {
		s.cleanupStoredAttachment(ctx, attachmentID, objectKey)
		return AttachmentResult{}, fmt.Errorf(
			"%w: %v",
			ErrAttachmentStorageUnavailable,
			err,
		)
	}
	return AttachmentResult{
		ID:          attachmentID,
		MessageType: messageTypeForMIME(command.ContentType),
		MIMEType:    command.ContentType,
		SizeBytes:   command.Size,
		URL:         downloadURL,
		Filename:    command.Filename,
	}, nil
}

// OpenAttachment 校验附件访问权限并返回对象内容流。
func (s *Service) OpenAttachment(
	ctx context.Context,
	userID uint64,
	attachmentID uint64,
) (OpenAttachmentResult, error) {
	if s.storage == nil {
		return OpenAttachmentResult{}, ErrAttachmentStorageUnavailable
	}
	if userID == 0 || attachmentID == 0 {
		return OpenAttachmentResult{}, ErrInvalidAttachment
	}

	record, err := s.repo.findAccessibleAttachment(
		ctx,
		userID,
		attachmentID,
	)
	if err != nil {
		return OpenAttachmentResult{}, err
	}
	reader, contentType, size, err := s.storage.OpenObject(ctx, record.ObjectKey)
	if err != nil {
		return OpenAttachmentResult{}, fmt.Errorf(
			"%w: %v",
			ErrAttachmentStorageUnavailable,
			err,
		)
	}
	return OpenAttachmentResult{
		Reader:    reader,
		MIMEType:  contentType,
		SizeBytes: size,
		Filename:  record.OriginalName,
	}, nil
}

// GetAttachmentInfo 校验权限并返回短时下载地址。
func (s *Service) GetAttachmentInfo(
	ctx context.Context,
	userID uint64,
	attachmentID uint64,
) (AttachmentResult, error) {
	if s.storage == nil {
		return AttachmentResult{}, ErrAttachmentStorageUnavailable
	}
	record, err := s.repo.findAccessibleAttachment(
		ctx,
		userID,
		attachmentID,
	)
	if err != nil {
		return AttachmentResult{}, err
	}
	downloadURL, err := s.storage.PresignedGetURL(
		ctx,
		record.ObjectKey,
		15*time.Minute,
	)
	if err != nil {
		return AttachmentResult{}, fmt.Errorf(
			"%w: %v",
			ErrAttachmentStorageUnavailable,
			err,
		)
	}
	return AttachmentResult{
		ID:          record.ID,
		MessageType: messageTypeForMIME(record.MIMEType),
		MIMEType:    record.MIMEType,
		SizeBytes:   record.SizeBytes,
		URL:         downloadURL,
		Filename:    record.OriginalName,
	}, nil
}

// safeFilename 清理上传文件名中的路径和非法字符
func safeFilename(filename string) string {
	filename = strings.TrimSpace(path.Base(strings.ReplaceAll(filename, "\\", "/")))
	filename = strings.ReplaceAll(filename, "\x00", "")
	if filename == "." || filename == "/" {
		return ""
	}
	if utf8.RuneCountInString(filename) > maxFilenameRunes {
		filename = string([]rune(filename)[:maxFilenameRunes])
	}
	return filename
}

// messageTypeForMIME 根据服务端检测到的 MIME 类型映射消息类型
func messageTypeForMIME(contentType string) int {
	if strings.HasPrefix(contentType, "image/") {
		return messageTypeImage
	}
	return messageTypeFile
}

// cleanupStoredAttachment 清理未被客户端拿到 ID 的失败上传。
// 清理使用脱离原请求取消信号的短超时上下文，避免请求断开后留下
// 没有可用响应的对象和附件元数据。
func (s *Service) cleanupStoredAttachment(
	ctx context.Context,
	attachmentID uint64,
	objectKey string,
) {
	cleanupCtx, cancel := context.WithTimeout(
		context.WithoutCancel(ctx),
		5*time.Second,
	)
	defer cancel()

	// 两项都是尽力清理；原始上传错误更重要，清理失败由后续孤儿数据
	// 巡检任务兜底，不在客户端主链路制造第二个错误。
	_ = s.storage.RemoveObject(cleanupCtx, objectKey)
	_ = s.repo.deleteAttachmentMetadata(cleanupCtx, attachmentID)
}
