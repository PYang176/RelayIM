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
	result      SendTextResult
	err         error
	recallErr   error
	calls       int
	recallCalls int
	command     SendTextCommand
	recallCmd   RecallMessageCommand
}

type stubMessageReader struct {
	page       MessagePage
	readStates []ReadStateResult
	delivery   DeliverySummary
	err        error
	calls      int
	command    ListMessagesCommand
}

func (s *stubMessageReader) ListMessages(
	_ context.Context,
	command ListMessagesCommand,
) (MessagePage, error) {
	s.calls++
	s.command = command
	return s.page, s.err
}

func (s *stubMessageReader) ListReadStates(
	_ context.Context,
	_ uint64,
	_ uint64,
) ([]ReadStateResult, error) {
	return s.readStates, s.err
}

func (s *stubMessageReader) GetDeliverySummary(
	_ context.Context,
	_ uint64,
	_ uint64,
	_ uint64,
) (DeliverySummary, error) {
	return s.delivery, s.err
}

func (s *stubTextMessageSender) SendText(
	_ context.Context,
	command SendTextCommand,
) (SendTextResult, error) {
	s.calls++
	s.command = command
	return s.result, s.err
}

func (s *stubTextMessageSender) SendAttachment(
	_ context.Context,
	command SendAttachmentCommand,
) (SendTextResult, error) {
	s.calls++
	s.command = SendTextCommand{
		ConversationID:  command.ConversationID,
		SenderID:        command.SenderID,
		ClientMessageID: command.ClientMessageID,
		Content:         command.Content,
	}
	return s.result, s.err
}

func (s *stubTextMessageSender) RecallMessage(
	_ context.Context,
	command RecallMessageCommand,
) error {
	s.recallCalls++
	s.recallCmd = command
	return s.recallErr
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

func TestHandlerRecallMessage(t *testing.T) {
	tests := []struct {
		name       string
		userID     uint64
		messageID  string
		err        error
		wantStatus int
		wantCalls  int
	}{
		{
			name:       "撤回成功",
			userID:     42,
			messageID:  "300",
			wantStatus: http.StatusNoContent,
			wantCalls:  1,
		},
		{
			name:       "缺少认证用户",
			messageID:  "300",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "消息 ID 非法",
			userID:     42,
			messageID:  "invalid",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "无权撤回",
			userID:     42,
			messageID:  "300",
			err:        ErrRecallForbidden,
			wantStatus: http.StatusForbidden,
			wantCalls:  1,
		},
		{
			name:       "超过撤回时间",
			userID:     42,
			messageID:  "300",
			err:        ErrRecallExpired,
			wantStatus: http.StatusConflict,
			wantCalls:  1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sender := &stubTextMessageSender{recallErr: tt.err}
			router := newMessagingHandlerRouter(sender, tt.userID)
			request := httptest.NewRequest(
				http.MethodDelete,
				"/api/v1/conversations/200/messages/"+tt.messageID,
				nil,
			)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)

			if response.Code != tt.wantStatus {
				t.Fatalf(
					"状态码 = %d，期望 %d，响应 = %s",
					response.Code,
					tt.wantStatus,
					response.Body.String(),
				)
			}
			if sender.recallCalls != tt.wantCalls {
				t.Fatalf(
					"RecallMessage 调用次数 = %d，期望 %d",
					sender.recallCalls,
					tt.wantCalls,
				)
			}
			if tt.wantCalls == 1 &&
				(sender.recallCmd.ConversationID != 200 ||
					sender.recallCmd.UserID != tt.userID ||
					sender.recallCmd.MessageID != 300) {
				t.Fatalf("撤回命令 = %+v", sender.recallCmd)
			}
		})
	}
}

func TestHandlerListMessages(t *testing.T) {
	createdAt := time.Date(2026, time.November, 7, 10, 0, 0, 0, time.UTC)
	replyTo := uint64(300)
	reader := &stubMessageReader{
		page: MessagePage{
			Messages: []MessageResult{
				{
					ID:               100,
					ConversationID:   200,
					SenderID:         42,
					Seq:              11,
					MessageType:      1,
					Content:          "after seq",
					ReplyToMessageID: &replyTo,
					CreatedAt:        createdAt,
				},
			},
			HasMore: true,
		},
	}
	router := newMessagingReaderRouter(reader, 42)

	response := performListMessagesRequest(
		router,
		"200",
		"after_seq=10&limit=2",
	)
	if response.Code != http.StatusOK {
		t.Fatalf("状态码 = %d，期望 200，响应 = %s",
			response.Code,
			response.Body.String(),
		)
	}
	if reader.calls != 1 ||
		reader.command.ConversationID != 200 ||
		reader.command.UserID != 42 ||
		reader.command.AfterSeq != 10 ||
		reader.command.Limit != 2 {
		t.Fatalf("历史消息命令 = %+v", reader.command)
	}

	var body struct {
		Messages []messageHistoryResponse `json:"messages"`
		HasMore  bool                     `json:"has_more"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("解析历史消息响应失败: %v", err)
	}
	if len(body.Messages) != 1 ||
		body.Messages[0].MessageID != "100" ||
		body.Messages[0].SenderID != "42" ||
		body.Messages[0].ReplyToMessageID == nil ||
		*body.Messages[0].ReplyToMessageID != "300" ||
		!body.HasMore {
		t.Fatalf("历史消息响应 = %+v", body)
	}
}

func TestHandlerListMessagesRejectsInvalidRequest(t *testing.T) {
	tests := []struct {
		name       string
		userID     uint64
		query      string
		readerErr  error
		wantStatus int
	}{
		{
			name:       "缺少认证用户",
			query:      "limit=20",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "游标格式错误",
			userID:     42,
			query:      "after_seq=invalid",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "分页数量格式错误",
			userID:     42,
			query:      "limit=invalid",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "非会话成员",
			userID:     42,
			query:      "limit=20",
			readerErr:  ErrNotMember,
			wantStatus: http.StatusForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := &stubMessageReader{err: tt.readerErr}
			router := newMessagingReaderRouter(reader, tt.userID)
			response := performListMessagesRequest(router, "200", tt.query)
			if response.Code != tt.wantStatus {
				t.Fatalf("状态码 = %d，期望 %d，响应 = %s",
					response.Code,
					tt.wantStatus,
					response.Body.String(),
				)
			}
		})
	}
}

func TestHandlerListReadStates(t *testing.T) {
	reader := &stubMessageReader{
		readStates: []ReadStateResult{
			{UserID: 42, LastReadSeq: 7},
			{UserID: 43, LastReadSeq: 5},
		},
	}
	router := newMessagingReaderRouter(reader, 42)
	request := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/conversations/200/read-states",
		nil,
	)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("状态码 = %d，期望 200，响应 = %s",
			response.Code,
			response.Body.String(),
		)
	}

	var body struct {
		ReadStates []readStateResponse `json:"read_states"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("解析已读状态失败: %v", err)
	}
	if len(body.ReadStates) != 2 ||
		body.ReadStates[0].UserID != "42" ||
		body.ReadStates[0].LastReadSeq != 7 {
		t.Fatalf("已读状态响应 = %+v", body)
	}
}

func TestHandlerGetDeliverySummary(t *testing.T) {
	reader := &stubMessageReader{
		delivery: DeliverySummary{
			DeliveredDevices: 2,
			DeliveredUsers:   1,
		},
	}
	router := newMessagingReaderRouter(reader, 42)
	request := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/conversations/200/messages/300/delivery",
		nil,
	)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("状态码 = %d，期望 200", response.Code)
	}
	var body struct {
		DeliveredDevices uint64 `json:"delivered_devices"`
		DeliveredUsers   uint64 `json:"delivered_users"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("解析送达汇总失败: %v", err)
	}
	if body.DeliveredDevices != 2 || body.DeliveredUsers != 1 {
		t.Fatalf("送达汇总 = %+v", body)
	}
}

func newMessagingHandlerRouter(
	sender MessageSender,
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

	handler := NewHandler(sender, nil, nil)
	router.POST(
		"/api/v1/conversations/:conversation_id/messages",
		handler.SendMessage,
	)
	router.DELETE(
		"/api/v1/conversations/:conversation_id/messages/:message_id",
		handler.RecallMessage,
	)
	return router
}

func newMessagingReaderRouter(
	reader MessageReader,
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

	handler := NewHandler(nil, reader, nil)
	router.GET(
		"/api/v1/conversations/:conversation_id/messages",
		handler.ListMessages,
	)
	router.GET(
		"/api/v1/conversations/:conversation_id/read-states",
		handler.ListReadStates,
	)
	router.GET(
		"/api/v1/conversations/:conversation_id/messages/:message_id/delivery",
		handler.GetDeliverySummary,
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

func performListMessagesRequest(
	router http.Handler,
	conversationID string,
	query string,
) *httptest.ResponseRecorder {
	request := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/conversations/"+conversationID+"/messages?"+query,
		nil,
	)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}
