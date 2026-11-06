package messaging

import (
	"context"
	"encoding/json"
	"errors"
	"ginChatProject/internal/middleware"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

type stubTextMessageSender struct {
	result  SendTextResult
	err     error
	calls   int
	command SendTextCommand
}

func (s *stubTextMessageSender) SendText(
	_ context.Context,
	command SendTextCommand,
) (SendTextResult, error) {
	s.calls++
	s.command = command
	return s.result, s.err
}

func TestHandlerSendText(t *testing.T) {
	createdAt := time.Date(2026, time.November, 7, 8, 30, 0, 0, time.UTC)
	tests := []struct {
		name               string
		userID             uint64
		conversationID     string
		body               string
		senderResult       SendTextResult
		senderErr          error
		wantStatus         int
		wantSenderCalls    int
		wantSenderID       uint64
		wantConversationID uint64
		wantReplyTo        uint64
		wantDuplicate      bool
	}{
		{
			name:           "首次发送返回201",
			userID:         42,
			conversationID: "200",
			body: `{
				"client_message_id": "client-1",
				"content": "你好",
				"reply_to_message_id": "300",
				"sender_id": "999"
			}`,
			senderResult: SendTextResult{
				MessageID:      100,
				ConversationID: 200,
				Seq:            7,
				CreatedAt:      createdAt,
			},
			wantStatus:         http.StatusCreated,
			wantSenderCalls:    1,
			wantSenderID:       42,
			wantConversationID: 200,
			wantReplyTo:        300,
		},
		{
			name:           "幂等重试返回200",
			userID:         42,
			conversationID: "200",
			body: `{
				"client_message_id": "client-1",
				"content": "你好"
			}`,
			senderResult: SendTextResult{
				MessageID:      100,
				ConversationID: 200,
				Seq:            7,
				CreatedAt:      createdAt,
				Duplicate:      true,
			},
			wantStatus:         http.StatusOK,
			wantSenderCalls:    1,
			wantSenderID:       42,
			wantConversationID: 200,
			wantDuplicate:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sender := &stubTextMessageSender{
				result: tt.senderResult,
				err:    tt.senderErr,
			}
			router := newMessagingHandlerRouter(sender, tt.userID)
			response := performSendTextRequest(
				router,
				tt.conversationID,
				tt.body,
			)

			if response.Code != tt.wantStatus {
				t.Fatalf("状态码 = %d，期望 %d，响应 = %s",
					response.Code,
					tt.wantStatus,
					response.Body.String(),
				)
			}
			if sender.calls != tt.wantSenderCalls {
				t.Fatalf("Service 调用次数 = %d，期望 %d", sender.calls, tt.wantSenderCalls)
			}
			if sender.command.SenderID != tt.wantSenderID {
				t.Fatalf("发送者 ID = %d，期望认证上下文中的 %d",
					sender.command.SenderID,
					tt.wantSenderID,
				)
			}
			if sender.command.ConversationID != tt.wantConversationID {
				t.Fatalf("会话 ID = %d，期望 %d",
					sender.command.ConversationID,
					tt.wantConversationID,
				)
			}
			if tt.wantReplyTo == 0 {
				if sender.command.ReplyToMessageID != nil {
					t.Fatalf("引用消息 ID = %v，期望 nil", sender.command.ReplyToMessageID)
				}
			} else if sender.command.ReplyToMessageID == nil ||
				*sender.command.ReplyToMessageID != tt.wantReplyTo {
				t.Fatalf("引用消息 ID = %v，期望 %d",
					sender.command.ReplyToMessageID,
					tt.wantReplyTo,
				)
			}

			var body sendTextResponse
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatalf("解析响应失败: %v", err)
			}
			if body.MessageID != "100" ||
				body.ConversationID != "200" ||
				body.Seq != 7 ||
				body.Duplicate != tt.wantDuplicate {
				t.Fatalf("响应 = %+v", body)
			}
		})
	}
}

func TestHandlerSendTextRejectsInvalidRequest(t *testing.T) {
	validBody := `{"client_message_id":"client-1","content":"hello"}`
	tests := []struct {
		name           string
		userID         uint64
		conversationID string
		body           string
		wantStatus     int
	}{
		{
			name:           "缺少认证用户",
			conversationID: "200",
			body:           validBody,
			wantStatus:     http.StatusUnauthorized,
		},
		{
			name:           "会话 ID 非法",
			userID:         42,
			conversationID: "invalid",
			body:           validBody,
			wantStatus:     http.StatusBadRequest,
		},
		{
			name:           "JSON 格式错误",
			userID:         42,
			conversationID: "200",
			body:           `{"content":`,
			wantStatus:     http.StatusBadRequest,
		},
		{
			name:           "引用消息 ID 非法",
			userID:         42,
			conversationID: "200",
			body: `{
				"client_message_id": "client-1",
				"content": "hello",
				"reply_to_message_id": "invalid"
			}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:           "请求体过大",
			userID:         42,
			conversationID: "200",
			body: `{"client_message_id":"client-1","content":"` +
				strings.Repeat("a", maxMessageRequestBodyBytes) +
				`"}`,
			wantStatus: http.StatusRequestEntityTooLarge,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sender := &stubTextMessageSender{}
			router := newMessagingHandlerRouter(sender, tt.userID)
			response := performSendTextRequest(
				router,
				tt.conversationID,
				tt.body,
			)

			if response.Code != tt.wantStatus {
				t.Fatalf("状态码 = %d，期望 %d，响应 = %s",
					response.Code,
					tt.wantStatus,
					response.Body.String(),
				)
			}
			if sender.calls != 0 {
				t.Fatalf("非法请求不应调用 Service，实际调用 %d 次", sender.calls)
			}
		})
	}
}

func TestHandlerSendTextErrorMapping(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
	}{
		{
			name:       "消息参数非法",
			err:        ErrInvalidMessage,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "引用消息非法",
			err:        ErrInvalidReply,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "非会话成员",
			err:        ErrNotMember,
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "会话不存在",
			err:        ErrConversationNotFound,
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "会话已关闭",
			err:        ErrConversationClosed,
			wantStatus: http.StatusConflict,
		},
		{
			name:       "客户端消息 ID 冲突",
			err:        ErrClientMessageConflict,
			wantStatus: http.StatusConflict,
		},
		{
			name:       "基础设施错误",
			err:        errors.New("database unavailable"),
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sender := &stubTextMessageSender{err: tt.err}
			router := newMessagingHandlerRouter(sender, 42)
			response := performSendTextRequest(
				router,
				"200",
				`{"client_message_id":"client-1","content":"hello"}`,
			)
			if response.Code != tt.wantStatus {
				t.Fatalf("状态码 = %d，期望 %d", response.Code, tt.wantStatus)
			}
		})
	}
}

func newMessagingHandlerRouter(
	sender TextMessageSender,
	userID uint64,
) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		if userID != 0 {
			c.Set(middleware.ContextUserIDKey, userID)
		}
		c.Next()
	})

	handler := NewHandler(sender)
	router.POST(
		"/api/v1/conversations/:conversation_id/messages",
		handler.SendText,
	)
	return router
}

func performSendTextRequest(
	router http.Handler,
	conversationID string,
	body string,
) *httptest.ResponseRecorder {
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/conversations/"+conversationID+"/messages",
		strings.NewReader(body),
	)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}
