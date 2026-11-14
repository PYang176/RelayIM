package conversation

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

type stubGroupAvatarStorage struct{}

// PutObject 模拟群头像对象上传
func (stubGroupAvatarStorage) PutObject(
	context.Context,
	string,
	io.Reader,
	int64,
	string,
) error {
	return nil
}

// OpenObject 模拟群头像对象读取
func (stubGroupAvatarStorage) OpenObject(
	context.Context,
	string,
) (io.ReadCloser, string, int64, error) {
	return io.NopCloser(strings.NewReader("")), "image/png", 0, nil
}

// RemoveObject 模拟群头像对象删除
func (stubGroupAvatarStorage) RemoveObject(context.Context, string) error {
	return nil
}

// TestGroupAvatarValidation 覆盖群头像上传、读取和删除的输入边界
func TestGroupAvatarValidation(t *testing.T) {
	service := NewServiceWithGroupAvatarStorage(
		nil,
		stubGroupAvatarStorage{},
	)
	tests := []struct {
		name    string
		run     func() error
		wantErr error
	}{
		{
			name: "上传操作人为空",
			run: func() error {
				_, err := service.UploadGroupAvatar(
					context.Background(),
					UploadGroupAvatarCommand{
						ConversationID: 1,
						ContentType:    "image/png",
						Size:           3,
						Reader:         strings.NewReader("png"),
					},
				)
				return err
			},
			wantErr: ErrInvalidGroupAvatar,
		},
		{
			name: "上传会话为空",
			run: func() error {
				_, err := service.UploadGroupAvatar(
					context.Background(),
					UploadGroupAvatarCommand{
						ActorID:     1,
						ContentType: "image/png",
						Size:        3,
						Reader:      strings.NewReader("png"),
					},
				)
				return err
			},
			wantErr: ErrInvalidGroupAvatar,
		},
		{
			name: "上传不支持的图片类型",
			run: func() error {
				_, err := service.UploadGroupAvatar(
					context.Background(),
					UploadGroupAvatarCommand{
						ActorID:        1,
						ConversationID: 2,
						ContentType:    "image/svg+xml",
						Size:           3,
						Reader:         strings.NewReader("svg"),
					},
				)
				return err
			},
			wantErr: ErrInvalidGroupAvatar,
		},
		{
			name: "上传图片超过大小限制",
			run: func() error {
				_, err := service.UploadGroupAvatar(
					context.Background(),
					UploadGroupAvatarCommand{
						ActorID:        1,
						ConversationID: 2,
						ContentType:    "image/png",
						Size:           maxGroupAvatarImageBytes + 1,
						Reader:         strings.NewReader("png"),
					},
				)
				return err
			},
			wantErr: ErrInvalidGroupAvatar,
		},
		{
			name: "读取会话为空",
			run: func() error {
				_, err := service.OpenGroupAvatar(
					context.Background(),
					0,
				)
				return err
			},
			wantErr: ErrInvalidInput,
		},
		{
			name: "删除操作人为空",
			run: func() error {
				_, err := service.RemoveGroupAvatar(
					context.Background(),
					0,
					1,
				)
				return err
			},
			wantErr: ErrInvalidInput,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.run(); !errors.Is(err, test.wantErr) {
				t.Fatalf("错误 = %v，期望 %v", err, test.wantErr)
			}
		})
	}
}

// TestGroupAvatarRequiresStorage 覆盖对象存储未配置时的明确失败语义
func TestGroupAvatarRequiresStorage(t *testing.T) {
	service := NewServiceWithGroupAvatarStorage(nil, nil)
	_, err := service.UploadGroupAvatar(
		context.Background(),
		UploadGroupAvatarCommand{
			ActorID:        1,
			ConversationID: 2,
			ContentType:    "image/png",
			Size:           3,
			Reader:         strings.NewReader("png"),
		},
	)
	if !errors.Is(err, ErrGroupAvatarStorageUnavailable) {
		t.Fatalf("错误 = %v，期望 ErrGroupAvatarStorageUnavailable", err)
	}
}
