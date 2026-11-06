package conversation

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestCreateDirectValidation(t *testing.T) {
	service := NewService(nil)
	tests := []struct {
		name    string
		command CreateDirectCommand
		wantErr error
	}{
		{
			name:    "当前用户 ID 为零",
			command: CreateDirectCommand{PeerUserID: 2},
			wantErr: ErrInvalidInput,
		},
		{
			name:    "对方用户 ID 为零",
			command: CreateDirectCommand{UserID: 1},
			wantErr: ErrInvalidInput,
		},
		{
			name: "不能与自己创建私聊",
			command: CreateDirectCommand{
				UserID:     1,
				PeerUserID: 1,
			},
			wantErr: ErrSameUser,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := service.CreateDirect(context.Background(), tt.command)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("CreateDirect() 错误 = %v，期望 %v", err, tt.wantErr)
			}
		})
	}
}

func TestCreateGroupValidation(t *testing.T) {
	service := NewService(nil)
	tests := []struct {
		name    string
		command CreateGroupCommand
	}{
		{
			name:    "群主 ID 为零",
			command: CreateGroupCommand{Name: "测试群"},
		},
		{
			name:    "群名称为空白",
			command: CreateGroupCommand{OwnerID: 1, Name: "  "},
		},
		{
			name: "群名称过长",
			command: CreateGroupCommand{
				OwnerID: 1,
				Name:    strings.Repeat("群", maxGroupNameRunes+1),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := service.CreateGroup(context.Background(), tt.command)
			if !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("CreateGroup() 错误 = %v，期望 ErrInvalidInput", err)
			}
		})
	}
}
