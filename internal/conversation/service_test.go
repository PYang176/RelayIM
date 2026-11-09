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

func TestConversationPaginationValidation(t *testing.T) {
	service := NewService(nil)
	tests := []struct {
		name string
		page ConversationPageQuery
	}{
		{
			name: "分页大小为零",
			page: ConversationPageQuery{},
		},
		{
			name: "分页大小超过上限",
			page: ConversationPageQuery{Limit: maxPageSize + 1},
		},
		{
			name: "搜索关键字过长",
			page: ConversationPageQuery{
				Limit:  defaultPageSize,
				Search: strings.Repeat("搜", maxConversationSearchRunes+1),
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := service.List(
				context.Background(),
				1,
				test.page,
			); !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("List() 错误 = %v，期望 ErrInvalidInput", err)
			}
		})
	}
}

func TestSetPinnedValidation(t *testing.T) {
	service := NewService(nil)
	tests := []struct {
		name           string
		userID         uint64
		conversationID uint64
	}{
		{name: "用户为空", conversationID: 1},
		{name: "会话为空", userID: 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := service.SetPinned(
				context.Background(),
				test.userID,
				test.conversationID,
				true,
			); !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("SetPinned() 错误 = %v，期望 ErrInvalidInput", err)
			}
		})
	}
}

func TestNormalizeGroupMemberIDs(t *testing.T) {
	tests := []struct {
		name    string
		ownerID uint64
		userIDs []uint64
		want    []uint64
		wantErr error
	}{
		{
			name:    "没有受邀成员",
			ownerID: 1,
			wantErr: ErrInvalidInput,
		},
		{
			name:    "包含群主自己",
			ownerID: 1,
			userIDs: []uint64{2, 1},
			wantErr: ErrInvalidInput,
		},
		{
			name:    "重复成员去重",
			ownerID: 1,
			userIDs: []uint64{2, 2, 3},
			want:    []uint64{2, 3},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeGroupMemberIDs(tt.ownerID, tt.userIDs)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("normalizeGroupMemberIDs() 错误 = %v，期望 %v", err, tt.wantErr)
			}
			if tt.wantErr == nil &&
				(len(got) != len(tt.want) ||
					got[0] != tt.want[0] ||
					got[1] != tt.want[1]) {
				t.Fatalf("normalizeGroupMemberIDs() = %v，期望 %v", got, tt.want)
			}
		})
	}
}

func TestInviteMembersValidation(t *testing.T) {
	service := NewService(nil)
	tests := []struct {
		name    string
		command InviteMembersCommand
	}{
		{
			name:    "邀请人为空",
			command: InviteMembersCommand{ConversationID: 1, MemberUserIDs: []uint64{2}},
		},
		{
			name:    "会话为空",
			command: InviteMembersCommand{ActorID: 1, MemberUserIDs: []uint64{2}},
		},
		{
			name: "邀请自己",
			command: InviteMembersCommand{
				ActorID:        1,
				ConversationID: 1,
				MemberUserIDs:  []uint64{1},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := service.InviteMembers(context.Background(), tt.command)
			if !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("InviteMembers() 错误 = %v，期望 ErrInvalidInput", err)
			}
		})
	}
}

func TestInvitationRemarkValidation(t *testing.T) {
	service := NewService(nil)
	if _, err := service.CreateGroup(
		context.Background(),
		CreateGroupCommand{
			OwnerID:       1,
			Name:          "测试群",
			MemberUserIDs: []uint64{2},
		},
	); !errors.Is(err, ErrInvitationRemarkRequired) {
		t.Fatalf("空建群邀请备注错误 = %v", err)
	}
	if _, err := service.CreateGroup(
		context.Background(),
		CreateGroupCommand{
			OwnerID:       1,
			Name:          "测试群",
			MemberUserIDs: []uint64{2},
			InviteRemark:  strings.Repeat("备", maxInvitationRemarkRunes+1),
		},
	); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("过长建群邀请备注错误 = %v", err)
	}
	if _, err := service.InviteMembers(
		context.Background(),
		InviteMembersCommand{
			ActorID:        1,
			ConversationID: 2,
			MemberUserIDs:  []uint64{3},
		},
	); !errors.Is(err, ErrInvitationRemarkRequired) {
		t.Fatalf("空管理员邀请备注错误 = %v", err)
	}
}

func TestGroupInvitationValidation(t *testing.T) {
	service := NewService(nil)
	tests := []struct {
		name string
		run  func() error
	}{
		{
			name: "接受邀请参数为空",
			run: func() error {
				_, err := service.AcceptInvitation(
					context.Background(),
					RespondInvitationCommand{InvitationID: 1},
				)
				return err
			},
		},
		{
			name: "拒绝邀请参数为空",
			run: func() error {
				return service.RejectInvitation(
					context.Background(),
					RespondInvitationCommand{UserID: 1},
				)
			},
		},
		{
			name: "收到邀请分页无效",
			run: func() error {
				_, err := service.ListReceivedInvitations(
					context.Background(),
					1,
					InvitationPageQuery{Limit: maxPageSize + 1},
				)
				return err
			},
		},
		{
			name: "群邀请分页无效",
			run: func() error {
				_, err := service.ListGroupInvitations(
					context.Background(),
					1,
					1,
					InvitationPageQuery{Limit: 0},
				)
				return err
			},
		},
		{
			name: "审批回复过长",
			run: func() error {
				_, err := service.AcceptInvitation(
					context.Background(),
					RespondInvitationCommand{
						UserID:         1,
						InvitationID:   2,
						ResponseRemark: strings.Repeat("复", maxInvitationResponseRunes+1),
					},
				)
				return err
			},
		},
		{
			name: "取消邀请参数为空",
			run: func() error {
				return service.CancelInvitation(
					context.Background(),
					CancelInvitationCommand{},
				)
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.run(); !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("错误 = %v，期望 ErrInvalidInput", err)
			}
		})
	}
}

func TestRemoveMemberValidation(t *testing.T) {
	service := NewService(nil)
	err := service.RemoveMember(
		context.Background(),
		RemoveMemberCommand{ActorID: 1, ConversationID: 1},
	)
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("RemoveMember() 错误 = %v，期望 ErrInvalidInput", err)
	}
}

func TestSetMemberRoleValidation(t *testing.T) {
	service := NewService(nil)
	tests := []struct {
		name    string
		command SetMemberRoleCommand
	}{
		{
			name: "操作人为空",
			command: SetMemberRoleCommand{
				ConversationID: 1,
				TargetUserID:   2,
				Role:           groupAdminRole,
			},
		},
		{
			name: "会话为空",
			command: SetMemberRoleCommand{
				ActorID:      1,
				TargetUserID: 2,
				Role:         groupAdminRole,
			},
		},
		{
			name: "目标成员为空",
			command: SetMemberRoleCommand{
				ActorID:        1,
				ConversationID: 1,
				Role:           groupAdminRole,
			},
		},
		{
			name: "不能直接设置群主角色",
			command: SetMemberRoleCommand{
				ActorID:        1,
				ConversationID: 1,
				TargetUserID:   2,
				Role:           groupOwnerRole,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := service.SetMemberRole(context.Background(), tt.command)
			if !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("SetMemberRole() 错误 = %v，期望 ErrInvalidInput", err)
			}
		})
	}
}

func TestTransferOwnerValidation(t *testing.T) {
	service := NewService(nil)
	tests := []struct {
		name    string
		command TransferOwnerCommand
	}{
		{
			name:    "群主为空",
			command: TransferOwnerCommand{ConversationID: 1, TargetUserID: 2},
		},
		{
			name:    "会话为空",
			command: TransferOwnerCommand{ActorID: 1, TargetUserID: 2},
		},
		{
			name: "受让人为空",
			command: TransferOwnerCommand{
				ActorID:        1,
				ConversationID: 1,
			},
		},
		{
			name: "不能转让给自己",
			command: TransferOwnerCommand{
				ActorID:        1,
				ConversationID: 1,
				TargetUserID:   1,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := service.TransferOwner(context.Background(), tt.command)
			if !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("TransferOwner() 错误 = %v，期望 ErrInvalidInput", err)
			}
		})
	}
}

func TestDissolveGroupValidation(t *testing.T) {
	service := NewService(nil)
	tests := []struct {
		name    string
		command DissolveGroupCommand
	}{
		{
			name:    "群主为空",
			command: DissolveGroupCommand{ConversationID: 1},
		},
		{
			name:    "会话为空",
			command: DissolveGroupCommand{ActorID: 1},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := service.DissolveGroup(context.Background(), tt.command)
			if !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("DissolveGroup() 错误 = %v，期望 ErrInvalidInput", err)
			}
		})
	}
}

func TestUpdateGroupProfileValidation(t *testing.T) {
	service := NewService(nil)
	tests := []struct {
		name    string
		command UpdateGroupProfileCommand
	}{
		{
			name: "群主为空",
			command: UpdateGroupProfileCommand{
				ConversationID: 1,
				Name:           "新群名",
			},
		},
		{
			name: "会话为空",
			command: UpdateGroupProfileCommand{
				ActorID: 1,
				Name:    "新群名",
			},
		},
		{
			name: "群名称为空",
			command: UpdateGroupProfileCommand{
				ActorID:        1,
				ConversationID: 1,
			},
		},
		{
			name: "群名称过长",
			command: UpdateGroupProfileCommand{
				ActorID:        1,
				ConversationID: 1,
				Name:           strings.Repeat("群", maxGroupNameRunes+1),
			},
		},
		{
			name: "群头像协议不安全",
			command: UpdateGroupProfileCommand{
				ActorID:        1,
				ConversationID: 1,
				Name:           "新群名",
				AvatarURL:      "javascript:alert(1)",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := service.UpdateGroupProfile(context.Background(), tt.command)
			if !errors.Is(err, ErrInvalidInput) {
				t.Fatalf(
					"UpdateGroupProfile() 错误 = %v，期望 ErrInvalidInput",
					err,
				)
			}
		})
	}
}
