package friendship

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestServiceValidation(t *testing.T) {
	service := NewService(nil)
	tests := []struct {
		name string
		run  func() error
	}{
		{
			name: "搜索词为空",
			run: func() error {
				_, err := service.SearchUsers(context.Background(), 1, " ")
				return err
			},
		},
		{
			name: "搜索词过长",
			run: func() error {
				_, err := service.SearchUsers(
					context.Background(),
					1,
					strings.Repeat("a", maxSearchQueryRunes+1),
				)
				return err
			},
		},
		{
			name: "不能添加自己",
			run: func() error {
				return service.SendRequest(context.Background(), 1, 1, "")
			},
		},
		{
			name: "申请附言过长",
			run: func() error {
				return service.SendRequest(
					context.Background(),
					1,
					2,
					strings.Repeat("附", maxRequestRemarkRunes+1),
				)
			},
		},
		{
			name: "好友备注过长",
			run: func() error {
				return service.SetRemark(
					context.Background(),
					1,
					2,
					strings.Repeat("备", maxFriendRemarkRunes+1),
				)
			},
		},
		{
			name: "不能拉黑自己",
			run: func() error {
				return service.BlockUser(context.Background(), 1, 1)
			},
		},
		{
			name: "不能取消拉黑自己",
			run: func() error {
				return service.UnblockUser(context.Background(), 1, 1)
			},
		},
		{
			name: "好友分页大小无效",
			run: func() error {
				_, err := service.ListFriends(
					context.Background(),
					1,
					PageQuery{Limit: 0},
				)
				return err
			},
		},
		{
			name: "申请分页超过上限",
			run: func() error {
				_, err := service.ListRequests(
					context.Background(),
					1,
					true,
					PageQuery{Limit: maxPageSize + 1},
				)
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.run(); !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("错误 = %v，期望 ErrInvalidInput", err)
			}
		})
	}
}

func TestSendRequestRequiresRemark(t *testing.T) {
	service := NewService(nil)
	err := service.SendRequest(context.Background(), 1, 2, " ")
	if !errors.Is(err, ErrRequestRemarkRequired) {
		t.Fatalf("错误 = %v，期望 ErrRequestRemarkRequired", err)
	}
}
