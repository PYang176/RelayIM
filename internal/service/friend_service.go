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

	// 检查双向是否有待处理申请
	pending1, _ := s.repo.HasPendingRequest(ctx, fromUserID, toUserID)
	pending2, _ := s.repo.HasPendingRequest(ctx, toUserID, fromUserID)
	if pending1 || pending2 {
		return errors.New("已存在待处理的好友申请")
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

// AcceptRequest 接受好友申请
func (s *FriendService) AcceptRequest(ctx context.Context, requestID uint, userID uint64) error {
	// 检查是否存在好友申请
	req, err := s.repo.FindByRequestID(ctx, requestID)
	if err != nil {
		return err
	}

	// 如果接受人ID与传进来的userID不匹配，则拒绝该申请
	if req.ToUserID != userID {
		return errors.New("无权操作此申请")
	}

	if req.Status != model.FriendRequestPending {
		return errors.New("处理的申请状态不正确")
	}

	// 创建双向好友关系
	err = s.repo.CreateRelation(ctx, req.FromUserID, req.ToUserID)
	if err != nil {
		return err
	}

	// 更改好友申请状态
	return s.repo.UpdateRequestStatus(ctx, requestID, model.FriendRequestAccepted)
}

// RejectRequest 拒绝好友申请
func (s *FriendService) RejectRequest(ctx context.Context, requestID uint, userID uint64) error {
	// 检查是否存在好友申请
	req, err := s.repo.FindByRequestID(ctx, requestID)
	if err != nil {
		return err
	}

	// 如果接受人ID与传进来的userID不匹配，则拒绝该申请
	if req.ToUserID != userID {
		return errors.New("无权操作此申请")
	}

	if req.Status != model.FriendRequestPending {
		return errors.New("处理的申请状态不正确")
	}

	// 更改好友申请状态
	return s.repo.UpdateRequestStatus(ctx, requestID, model.FriendRequestRejected)
}

// ListReceiveFriendRequest 获取自己收到的好友申请列表
func (s *FriendService) ListReceiveFriendRequest(ctx context.Context, userID uint) ([]*model.FriendRequest, error) {
	return s.repo.GetReceiveRequests(ctx, userID)
}

// ListSendFriendRequest 获取自己发出的好友申请列表
func (s *FriendService) ListSendFriendRequest(ctx context.Context, userID uint) ([]*model.FriendRequest, error) {
	return s.repo.GetSentRequests(ctx, userID)
}

// ListFriends 获取好友列表
func (s *FriendService) ListFriends(ctx context.Context, userID uint64) ([]*model.FriendRelation, error) {
	return s.repo.ListFriends(ctx, userID)
}

// DeleteFriend 删除好友
func (s *FriendService) DeleteFriend(ctx context.Context, userID, friendID uint64) error {
	// 检查两人是否是好友
	isFriend, err := s.repo.CheckFriendShip(ctx, userID, friendID)
	if err != nil {
		return err
	}
	if !isFriend {
		return errors.New("双方不是好友关系")
	}
	// 删除好友
	return s.repo.DeleteFriend(ctx, userID, friendID)
}
