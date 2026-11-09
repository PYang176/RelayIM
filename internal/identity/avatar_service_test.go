package identity

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
)

type memoryAvatarStorage struct {
	mu          sync.Mutex
	objects     map[string][]byte
	types       map[string]string
	removedKeys []string
}

func newMemoryAvatarStorage() *memoryAvatarStorage {
	return &memoryAvatarStorage{
		objects: make(map[string][]byte),
		types:   make(map[string]string),
	}
}

func (s *memoryAvatarStorage) PutObject(
	ctx context.Context,
	objectKey string,
	reader io.Reader,
	size int64,
	contentType string,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	value, err := io.ReadAll(reader)
	if err != nil {
		return err
	}
	if int64(len(value)) != size {
		return errors.New("测试对象大小不一致")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.objects[objectKey] = value
	s.types[objectKey] = contentType
	return nil
}

func (s *memoryAvatarStorage) OpenObject(
	ctx context.Context,
	objectKey string,
) (io.ReadCloser, string, int64, error) {
	if err := ctx.Err(); err != nil {
		return nil, "", 0, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	value, exists := s.objects[objectKey]
	if !exists {
		return nil, "", 0, errors.New("测试对象不存在")
	}
	return io.NopCloser(bytes.NewReader(value)),
		s.types[objectKey],
		int64(len(value)),
		nil
}

func (s *memoryAvatarStorage) RemoveObject(
	ctx context.Context,
	objectKey string,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.objects, objectKey)
	delete(s.types, objectKey)
	s.removedKeys = append(s.removedKeys, objectKey)
	return nil
}

func TestServiceAvatarValidation(t *testing.T) {
	service := NewServiceWithAvatarStorage(nil, nil, newMemoryAvatarStorage())
	tests := []struct {
		name    string
		command UploadAvatarCommand
		wantErr error
	}{
		{
			name: "缺少用户",
			command: UploadAvatarCommand{
				ContentType: "image/png",
				Size:        3,
				Reader:      strings.NewReader("png"),
			},
			wantErr: ErrInvalidAvatar,
		},
		{
			name: "不支持的类型",
			command: UploadAvatarCommand{
				UserID:      1,
				ContentType: "image/svg+xml",
				Size:        3,
				Reader:      strings.NewReader("svg"),
			},
			wantErr: ErrInvalidAvatar,
		},
		{
			name: "文件过大",
			command: UploadAvatarCommand{
				UserID:      1,
				ContentType: "image/png",
				Size:        maxAvatarBytes + 1,
				Reader:      strings.NewReader("png"),
			},
			wantErr: ErrInvalidAvatar,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := service.UploadAvatar(
				context.Background(),
				test.command,
			)
			if !errors.Is(err, test.wantErr) {
				t.Fatalf(
					"UploadAvatar() 错误 = %v，期望 %v",
					err,
					test.wantErr,
				)
			}
		})
	}

	if _, err := NewServiceWithAvatarStorage(nil, nil, nil).
		UploadAvatar(context.Background(), UploadAvatarCommand{}); !errors.Is(
		err,
		ErrAvatarStorageUnavailable,
	) {
		t.Fatalf("未配置头像存储错误 = %v", err)
	}
}
