package friendship

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestParsePageQuery 覆盖好友分页查询参数的解析和边界
func TestParsePageQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name       string
		target     string
		wantLimit  int
		wantBefore uint64
		wantOK     bool
	}{
		{
			name:      "默认分页",
			target:    "/friends",
			wantLimit: defaultPageSize,
			wantOK:    true,
		},
		{
			name:       "游标分页",
			target:     "/friends?limit=50&before_id=123",
			wantLimit:  50,
			wantBefore: 123,
			wantOK:     true,
		},
		{
			name:   "数量超过上限",
			target: "/friends?limit=101",
		},
		{
			name:   "游标格式错误",
			target: "/friends?before_id=invalid",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(recorder)
			context.Request = httptest.NewRequest("GET", test.target, nil)

			page, ok := parsePageQuery(context)
			if ok != test.wantOK {
				t.Fatalf("ok = %v，期望 %v", ok, test.wantOK)
			}
			if !ok {
				if recorder.Code != 400 {
					t.Fatalf("状态码 = %d，期望 400", recorder.Code)
				}
				return
			}
			if page.Limit != test.wantLimit || page.BeforeID != test.wantBefore {
				t.Fatalf("分页参数 = %+v", page)
			}
		})
	}
}
