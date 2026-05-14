package service

import (
	"context"
	"errors"
	"ginChatProject/internal/model"
	"ginChatProject/internal/repository"
	"ginChatProject/utils/snowflake"
)

type MessageService struct {
	repo *repository.MessageRepository
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

func NewMessageService(repo *repository.MessageRepository) *MessageService {
	return &MessageService{repo: repo}
}

// Send 发送消息总方法，根据消息类型进行处理
func (s *MessageService) Send(ctx context.Context, params SendMessageParams) (string, error) {
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

	msg := &model.Message{
		MessageID:  snowflake.GenerateString(),
		SenderID:   params.SenderID,
		ReceiverID: params.ReceiverID,
		SendType:   params.SendType,
		MsgType:    params.MsgType,
		Content:    params.Content,
		URL:        params.URL,
		Status:     model.MsgSent,
	}
	return msg.MessageID, s.repo.Create(ctx, msg)
}
