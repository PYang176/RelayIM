package messaging

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestServiceSendTextValidation(t *testing.T) {
	service := NewService(nil)
	replyID := uint64(0)

	tests := []struct {
		name    string
		command SendTextCommand
	}{
		{
			name: "缺少会话 ID",
			command: SendTextCommand{
				SenderID:        1,
				ClientMessageID: "client-1",
				Content:         "hello",
			},
		},
		{
			name: "缺少发送者 ID",
			command: SendTextCommand{
				ConversationID:  1,
				ClientMessageID: "client-1",
				Content:         "hello",
			},
		},
		{
			name: "客户端消息 ID 为空",
			command: SendTextCommand{
				ConversationID: 1,
				SenderID:       1,
				Content:        "hello",
			},
		},
		{
			name: "消息内容为空",
			command: SendTextCommand{
				ConversationID:  1,
				SenderID:        1,
				ClientMessageID: "client-1",
				Content:         "   ",
			},
		},
		{
			name: "消息内容过长",
			command: SendTextCommand{
				ConversationID:  1,
				SenderID:        1,
				ClientMessageID: "client-1",
				Content:         strings.Repeat("测", maxTextContentRunes+1),
			},
		},
		{
			name: "引用消息 ID 非法",
			command: SendTextCommand{
				ConversationID:   1,
				SenderID:         1,
				ClientMessageID:  "client-1",
				Content:          "hello",
				ReplyToMessageID: &replyID,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := service.SendText(context.Background(), tt.command)
			if !errors.Is(err, ErrInvalidMessage) {
				t.Fatalf("SendText() 错误 = %v，期望 ErrInvalidMessage", err)
			}
		})
	}
}

func TestServiceListMessagesValidation(t *testing.T) {
	service := NewService(nil)
	tests := []struct {
		name    string
		command ListMessagesCommand
	}{
		{
			name:    "缺少会话 ID",
			command: ListMessagesCommand{UserID: 1},
		},
		{
			name:    "缺少用户 ID",
			command: ListMessagesCommand{ConversationID: 1},
		},
		{
			name: "同时使用前后游标",
			command: ListMessagesCommand{
				ConversationID: 1,
				UserID:         1,
				AfterSeq:       1,
				BeforeSeq:      10,
			},
		},
		{
			name: "分页数量超过上限",
			command: ListMessagesCommand{
				ConversationID: 1,
				UserID:         1,
				Limit:          maxHistoryLimit + 1,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := service.ListMessages(context.Background(), tt.command)
			if !errors.Is(err, ErrInvalidMessage) {
				t.Fatalf("ListMessages() 错误 = %v，期望 ErrInvalidMessage", err)
			}
		})
	}
}
