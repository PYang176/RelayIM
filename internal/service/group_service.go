package service

import (
	"context"
	"errors"
	"ginChatProject/internal/model"
	"ginChatProject/internal/repository"
	"time"
)

type GroupService struct {
	repo *repository.GroupRepository
}

func NewGroupService(repo *repository.GroupRepository) *GroupService {
	return &GroupService{repo: repo}
}

// CreateGroup 创建群聊
func (s *GroupService) CreateGroup(ctx context.Context, ownerID uint64, name string, avatar string) (*model.GroupBasic, error) {
	// 创建群聊基本信息
	group := &model.GroupBasic{
		Name:      name,
		Avatar:    avatar,
		OwnerID:   ownerID,
		GroupType: model.GroupNormal,
		Status:    model.GroupActive,
	}
	if err := s.repo.Create(ctx, group); err != nil {
		return nil, errors.New("创建群聊失败")
	}

	// 将群主添加为群成员
	member := model.GroupMember{
		GroupID:  uint64(group.ID),
		UserID:   ownerID,
		Role:     model.GroupOwnerRole,
		JoinTime: time.Now(),
	}
	err := s.repo.AddMember(ctx, &member)
	if err != nil {
		return nil, errors.New("添加群主失败")
	}

	return group, nil
}

// JoinGroup 加入群聊
func (s *GroupService) JoinGroup(ctx context.Context, groupID, userID uint64, nickname string) error {
	// 1. 检查群是否存在
	group, err := s.repo.GetGroupByID(ctx, groupID)
	if err != nil {
		return errors.New("群聊不存在")
	}

	// 2. 检查群状态
	if group.Status == model.GroupDissolved {
		return errors.New("群聊已解散")
	}

	// 3. 检查是否已加入
	isMember, err := s.repo.CheckMembership(ctx, groupID, userID)
	if err != nil {
		return err
	}
	if isMember {
		return errors.New("已在群聊中")
	}

	// 4. 添加群成员
	member := &model.GroupMember{
		GroupID:  groupID,
		UserID:   userID,
		Role:     model.GroupMemberRole,
		Nickname: nickname,
		JoinTime: time.Now(),
	}

	return s.repo.AddMember(ctx, member)
}

// LeaveGroup 退出群聊
func (s *GroupService) LeaveGroup(ctx context.Context, groupID, userID uint64) error {
	// 1. 检查是否是群主
	group, err := s.repo.GetGroupByID(ctx, groupID)
	if err != nil {
		return errors.New("群聊不存在")
	}

	if group.OwnerID == userID {
		return errors.New("群主不能退出群聊")
	}

	// 2. 检查是否在群中
	isMember, err := s.repo.CheckMembership(ctx, groupID, userID)
	if err != nil || !isMember {
		return errors.New("不在群聊中")
	}

	// 3. 移除群成员
	return s.repo.RemoveMember(ctx, groupID, userID)
}

// GetMyGroups 获取我加入的群列表
func (s *GroupService) GetMyGroups(ctx context.Context, userID uint64) ([]*model.GroupBasic, error) {
	return s.repo.GetUserGroups(ctx, userID)
}

// GetGroupMembers 获取群成员列表
func (s *GroupService) GetGroupMembers(ctx context.Context, groupID uint64) ([]*model.GroupMember, error) {
	return s.repo.GetGroupMembers(ctx, groupID)
}
