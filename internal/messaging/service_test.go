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

func TestServiceSearchMessagesValidation(t *testing.T) {
	service := NewService(nil)
	tests := []struct {
		name    string
		command SearchMessagesCommand
		wantErr error
	}{
		{
			name:    "缺少会话 ID",
			command: SearchMessagesCommand{UserID: 1, Query: "消息"},
			wantErr: ErrInvalidMessage,
		},
		{
			name: "搜索词过短",
			command: SearchMessagesCommand{
				ConversationID: 1,
				UserID:         1,
				Query:          "搜",
			},
			wantErr: ErrInvalidSearchQuery,
		},
		{
			name: "搜索词过长",
			command: SearchMessagesCommand{
				ConversationID: 1,
				UserID:         1,
				Query:          strings.Repeat("搜", maxSearchQueryRunes+1),
			},
			wantErr: ErrInvalidSearchQuery,
		},
		{
			name: "分页数量超过上限",
			command: SearchMessagesCommand{
				ConversationID: 1,
				UserID:         1,
				Query:          "消息",
				Limit:          maxHistoryLimit + 1,
			},
			wantErr: ErrInvalidMessage,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := service.SearchMessages(
				context.Background(),
				test.command,
			)
			if !errors.Is(err, test.wantErr) {
				t.Fatalf(
					"SearchMessages() 错误 = %v，期望 %v",
					err,
					test.wantErr,
				)
			}
		})
	}
}

func TestServiceReceiptValidation(t *testing.T) {
	service := NewService(nil)
	if err := service.AcknowledgeMessage(
		context.Background(),
		0,
		1,
		1,
	); !errors.Is(err, ErrInvalidMessage) {
		t.Fatalf("ACK 参数错误 = %v，期望 ErrInvalidMessage", err)
	}
	if err := service.MarkConversationRead(
		context.Background(),
		1,
		0,
		1,
	); !errors.Is(err, ErrInvalidMessage) {
		t.Fatalf("已读参数错误 = %v，期望 ErrInvalidMessage", err)
	}
}

func TestServiceRecallValidation(t *testing.T) {
	service := NewService(nil)
	tests := []RecallMessageCommand{
		{UserID: 1, MessageID: 2},
		{ConversationID: 1, MessageID: 2},
		{ConversationID: 1, UserID: 1},
	}
	for index, command := range tests {
		if err := service.RecallMessage(
			context.Background(),
			command,
		); !errors.Is(err, ErrInvalidMessage) {
			t.Fatalf(
				"第 %d 个撤回参数错误 = %v，期望 ErrInvalidMessage",
				index,
				err,
			)
		}
	}
}
