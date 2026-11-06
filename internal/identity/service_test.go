package identity

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestRegisterValidation(t *testing.T) {
	service := NewService(nil, nil)

	tests := []struct {
		name    string
		command RegisterCommand
	}{
		{
			name: "用户名过短",
			command: RegisterCommand{
				Username:        "a",
				Password:        "password123",
				ConfirmPassword: "password123",
			},
		},
		{
			name: "用户名包含空格",
			command: RegisterCommand{
				Username:        "bad name",
				Password:        "password123",
				ConfirmPassword: "password123",
			},
		},
		{
			name: "密码过短",
			command: RegisterCommand{
				Username:        "valid_user",
				Password:        "short",
				ConfirmPassword: "short",
			},
		},
		{
			name: "两次密码不一致",
			command: RegisterCommand{
				Username:        "valid_user",
				Password:        "password123",
				ConfirmPassword: "password456",
			},
		},
		{
			name: "密码超过 bcrypt 上限",
			command: RegisterCommand{
				Username:        "valid_user",
				Password:        strings.Repeat("a", maxPasswordBytes+1),
				ConfirmPassword: strings.Repeat("a", maxPasswordBytes+1),
			},
		},
		{
			name: "昵称过长",
			command: RegisterCommand{
				Username:        "valid_user",
				Password:        "password123",
				ConfirmPassword: "password123",
				Nickname:        strings.Repeat("昵", maxNicknameRunes+1),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := service.Register(context.Background(), tt.command)
			if !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("Register() 错误 = %v，期望 ErrInvalidInput", err)
			}
		})
	}
}

func TestLoginValidation(t *testing.T) {
	service := NewService(nil, nil)

	tests := []struct {
		name    string
		command LoginCommand
	}{
		{
			name:    "用户名为空",
			command: LoginCommand{Password: "password123"},
		},
		{
			name:    "密码为空",
			command: LoginCommand{Username: "valid_user"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := service.Login(context.Background(), tt.command)
			if !errors.Is(err, ErrInvalidCredentials) {
				t.Fatalf("Login() 错误 = %v，期望 ErrInvalidCredentials", err)
			}
		})
	}
}
