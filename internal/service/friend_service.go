package service

import (
	"context"
	"errors"
	"ginChatProject/internal/model"
	"ginChatProject/internal/repository"
)

type FriendService struct {
	repo *repository.FriendRepository
}

func NewFriendService(repo *repository.FriendRepository) *FriendService {
	return &FriendService{repo: repo}
}

// SendRequest 发送好友申请
func (s *FriendService) SendRequest(ctx context.Context, fromUserID uint64, toUserID uint64, remark string) error {
	// 不能自己添加自己
	if fromUserID == toUserID {
		return errors.New("不能添加自己为好友")
	}

	// 检查对方是否存在
	exists, err := s.repo.UserExists(ctx, toUserID)
	if err != nil {
		return err
	}
	if !exists {
		return errors.New("添加的用户不存在")
	}

	// 是否已经是好友
	isFriend, err := s.repo.CheckFriendShip(ctx, fromUserID, toUserID)
	if err != nil {
		return err
	}
	if isFriend {
		return errors.New("已经是好友了，不能重复添加")
	}

	// 是否有待处理的好友申请
	isPending, err := s.repo.HasPendingRequest(ctx, fromUserID, toUserID)
	if err != nil {
		return err
	}
	if isPending {
		return errors.New("已经提交过好友申请啦，请等待对方处理")
	}

	// 构建好友申请的参数
	req := &model.FriendRequest{
		FromUserID: fromUserID,
		ToUserID:   toUserID,
		Status:     model.FriendRequestPending,
		Remark:     remark,
	}
	return s.repo.CreateRequest(ctx, req)
}
