package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"ginChatProject/internal/model"
	"ginChatProject/internal/repository"
	"ginChatProject/internal/workerpool"
	"ginChatProject/utils/redis_util"
	"ginChatProject/utils/snowflake"
	"time"
)

type MessageService struct {
	repo       *repository.MessageRepository
	friendRepo *repository.FriendRepository
	groupRepo  *repository.GroupRepository
	workerPool *workerpool.WorkerPool // 线程池
}

// SendMessageParams 发送消息参数
type SendMessageParams struct {
	SenderID   uint64
	ReceiverID uint64
	SendType   model.SendType
	MsgType    model.MsgType
	Content    string
	URL        string
}

// SendGroupMessageParams 发送群消息参数
type SendGroupMessageParams struct {
	GroupID  uint64
	SenderID uint64
	MsgType  model.MsgType
	Content  string
	URL      string
}

func NewMessageService(repo *repository.MessageRepository,
	friendRepo *repository.FriendRepository,
	groupRepo *repository.GroupRepository,
) *MessageService {
	return &MessageService{repo: repo,
		friendRepo: friendRepo,
		groupRepo:  groupRepo}
}

// Send 发送消息总方法，根据消息类型进行处理
func (s *MessageService) Send(ctx context.Context, params SendMessageParams) (string, error) {
	// 私聊时检查好友关系
	if params.SendType == model.SendPrivate {
		isFriend, err := s.friendRepo.CheckFriendShip(
			ctx,
			params.SenderID,
			params.ReceiverID,
		)
		if err != nil {
			return "", err
		}
		if !isFriend {
			return "", errors.New("非好友关系，无法发送消息")
		}
	}

	switch params.MsgType {
	case model.MsgText:
		return s.sendText(ctx, params)
	case model.MsgImage, model.MsgFile, model.MsgAudio:
		//return s.sendMedia(ctx, params)
	default:
		return "", errors.New("unsupported message type")
	}
	return "", errors.New("unsupported message type")
}

// 文本消息发送
func (s *MessageService) sendText(ctx context.Context, params SendMessageParams) (string, error) {
	// 设置默认值
	if params.MsgType == 0 {
		params.MsgType = model.MsgText
	}
	if params.SendType == 0 {
		params.SendType = model.SendPrivate
	}

	// 构造消息参数
	msg := &model.Message{
		MessageID:  snowflake.GenerateString(),
		SenderID:   params.SenderID,
		ReceiverID: params.ReceiverID,
		SendType:   params.SendType,
		MsgType:    params.MsgType,
		Content:    params.Content,
		URL:        params.URL,
		Status:     model.MsgSent, // 默认为已发送
	}

	// 判断是否在线
	//if s.onlineChecker.IsOnline(uint(params.ReceiverID)) {
	//	msg.Status = model.MsgDelivered // 在线 → 已送达
	//} else {
	//	msg.Status = model.MsgUndelivered // 离线 → 未送达
	//}
	msg.Status = model.MsgUndelivered // 离线 → 未送达
	// 3. 异步入库（不阻塞响应）
	go func() {
		s.repo.Create(context.Background(), msg)
	}()

	// 4. 立即返回
	return msg.MessageID, nil
}

// PullOfflineMessages 拉取离线消息
func (s *MessageService) PullOfflineMessages(ctx context.Context, userID uint) ([]*model.Message, error) {
	msgs, err := s.repo.GetOfflineMessages(ctx, userID)
	if err != nil {
		return nil, err
	}

	return msgs, nil
}

// AckMessage 客户端消息确认
func (s *MessageService) AckMessage(ctx context.Context, messageID string, userID uint) error {
	// 检查ack的userID是否与receiverID相符
	message, err := s.repo.FindByMessageID(ctx, messageID)
	if err != nil {
		return err
	}
	if message.ReceiverID != uint64(userID) {
		return errors.New("非法的ACK")
	}
	return s.repo.UpdateStatus(ctx, messageID, model.MsgDelivered)
}

// ReadMessage 消息已读
func (s *MessageService) ReadMessage(ctx context.Context, messageID string, userID uint) error {
	// 检查ack的userID是否与receiverID相符
	message, err := s.repo.FindByMessageID(ctx, messageID)
	if err != nil {
		return err
	}
	if message.ReceiverID != uint64(userID) {
		return errors.New("非法的ACK")
	}
	now := time.Now()
	updates := repository.MessageUpdateFields{
		MessageID: messageID,
		Status:    model.MsgRead,
		ReadTime:  &now,
	}

	return s.repo.UpdateStatusAndTime(
		ctx,
		updates,
	)
}

// SendGroupMessage 群消息发送
func (s *MessageService) SendGroupMessage(ctx context.Context, params SendGroupMessageParams) (*model.Message, error) {
	// 1. 检查是否在群里
	isMember, err := s.groupRepo.CheckMembership(ctx, params.GroupID, params.SenderID)
	if err != nil || !isMember {
		return nil, errors.New("不在群聊中")
	}

	// 2. 创建消息
	msg := &model.Message{
		MessageID:  snowflake.GenerateString(),
		SenderID:   params.SenderID,
		ReceiverID: params.GroupID,
		SendType:   model.SendGroup,
		MsgType:    params.MsgType,
		Content:    params.Content,
		URL:        params.URL,
		Status:     model.MsgUndelivered,
	}

	if err := s.repo.Create(ctx, msg); err != nil {
		return nil, err
	}

	// 3. 获取需要推送的用户列表
	//members, err := s.groupRepo.GetGroupMembers(ctx, params.GroupID)
	//if err != nil {
	//	return nil, nil, err
	//}
	//
	//userIDs := make([]uint64, 0)
	//for _, member := range members {
	//	if member.UserID != params.SenderID {
	//		userIDs = append(userIDs, member.UserID)
	//	}
	//}

	// 3.  发送到 Redis Stream（异步）
	go func() {
		payload, _ := json.Marshal(msg)
		_, err := redis_util.ProduceMessage(context.Background(), redis_util.GroupMsgStream, map[string]interface{}{
			"message_id": msg.MessageID,
			"payload":    string(payload),
		})
		if err != nil {
			fmt.Printf("发送消息到 Stream 失败: %v\n", err)
		}
	}()

	return msg, nil
}
