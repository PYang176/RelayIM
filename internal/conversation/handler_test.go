package conversation

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// TestParseConversationPageQuery 覆盖会话分页查询参数的解析和边界
func TestParseConversationPageQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	validCursor := encodeConversationCursor(
		true,
		time.Date(2026, 11, 10, 8, 30, 0, 123000000, time.UTC),
		1839281749283749120,
	)
	tests := []struct {
		name       string
		query      string
		wantStatus int
		wantLimit  int
		wantCursor bool
	}{
		{
			name:       "使用默认分页大小",
			wantStatus: 200,
			wantLimit:  defaultPageSize,
		},
		{
			name:       "接受合法分页参数",
			query:      "?q=项目&limit=50&cursor=" + validCursor,
			wantStatus: 200,
			wantLimit:  50,
			wantCursor: true,
		},
		{
			name:       "拒绝非数字上限",
			query:      "?limit=abc",
			wantStatus: 400,
		},
		{
			name:       "拒绝零上限",
			query:      "?limit=0",
			wantStatus: 400,
		},
		{
			name:       "拒绝超限分页大小",
			query:      "?limit=101",
			wantStatus: 400,
		},
		{
			name:       "拒绝损坏游标",
			query:      "?cursor=not-a-cursor",
			wantStatus: 400,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(recorder)
			context.Request = httptest.NewRequest(
				"GET",
				"/api/v1/conversations"+test.query,
				nil,
			)

			page, ok := parseConversationPageQuery(context)
			status := recorder.Code
			if ok {
				status = 200
			}
			if status != test.wantStatus {
				t.Fatalf("状态码 = %d，期望 %d", status, test.wantStatus)
			}
			if !ok {
				return
			}
			if page.Limit != test.wantLimit {
				t.Fatalf("limit = %d，期望 %d", page.Limit, test.wantLimit)
			}
			if test.wantCursor && page.Search != "项目" {
				t.Fatalf("搜索词 = %q，期望 项目", page.Search)
			}
			if test.wantCursor &&
				(!page.BeforePinned ||
					page.BeforeUpdatedAt.IsZero() ||
					page.BeforeID == 0) {
				t.Fatalf("游标未解析: %+v", page)
			}
		})
	}
}

// TestConversationCursorRoundTrip 覆盖会话游标编码和解码后的一致性
func TestConversationCursorRoundTrip(t *testing.T) {
	wantTime := time.Date(2026, 11, 10, 8, 30, 0, 123000000, time.UTC)
	const wantID uint64 = 1839281749283749120

	cursor := encodeConversationCursor(true, wantTime, wantID)
	gotPinned, gotTime, gotID, err := decodeConversationCursor(cursor)
	if err != nil {
		t.Fatalf("解析会话游标失败: %v", err)
	}
	if !gotPinned || !gotTime.Equal(wantTime) || gotID != wantID {
		t.Fatalf(
			"会话游标 = (%t, %s, %d)，期望 (true, %s, %d)",
			gotPinned,
			gotTime,
			gotID,
			wantTime,
			wantID,
		)
	}
}

// TestConversationCursorRejectsInvalidPayload 覆盖非法会话游标载荷的拒绝路径
func TestConversationCursorRejectsInvalidPayload(t *testing.T) {
	invalid := []string{
		"",
		"not-base64!",
		"MTIz",
		"MHwx",
		"TXwxMjN8MQ==",
		"MHwwfDE=",
		"MHwxMjN8MA==",
		"MHwxMjN8YWJj",
	}
	for _, value := range invalid {
		t.Run(value, func(t *testing.T) {
			if _, _, _, err := decodeConversationCursor(value); err == nil {
				t.Fatal("无效游标未被拒绝")
			}
		})
	}
}
