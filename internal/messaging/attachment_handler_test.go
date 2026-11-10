package messaging

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"ginChatProject/internal/middleware"
	"github.com/gin-gonic/gin"
)

type stubAttachmentManager struct {
	uploadCalls int
}

// UploadAttachment 模拟附件上传并记录收到的命令
func (s *stubAttachmentManager) UploadAttachment(
	_ context.Context,
	_ UploadAttachmentCommand,
) (AttachmentResult, error) {
	s.uploadCalls++
	return AttachmentResult{}, nil
}

// OpenAttachment 返回测试附件对象读取器，模拟对象存储下载
func (s *stubAttachmentManager) OpenAttachment(
	_ context.Context,
	_ uint64,
	_ uint64,
) (OpenAttachmentResult, error) {
	return OpenAttachmentResult{}, nil
}

// GetAttachmentInfo 读取附件Info
func (s *stubAttachmentManager) GetAttachmentInfo(
	_ context.Context,
	_ uint64,
	_ uint64,
) (AttachmentResult, error) {
	return AttachmentResult{}, nil
}

// TestUploadAttachmentRejectsOversizedFile 覆盖超大附件在上传前被拒绝
func TestUploadAttachmentRejectsOversizedFile(t *testing.T) {
	gin.SetMode(gin.TestMode)
	attachments := &stubAttachmentManager{}
	handler := NewHandler(nil, nil, attachments)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(middleware.ContextUserIDKey, uint64(42))
		c.Next()
	})
	router.POST("/api/v1/attachments", handler.UploadAttachment)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	filePart, err := writer.CreateFormFile("file", "too-large.bin")
	if err != nil {
		t.Fatalf("创建 multipart 文件失败: %v", err)
	}
	if _, err := filePart.Write(make([]byte, maxAttachmentBytes+1)); err != nil {
		t.Fatalf("写入测试文件失败: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("关闭 multipart 失败: %v", err)
	}

	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/attachments",
		&body,
	)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf(
			"状态码 = %d，期望 413，响应 = %s",
			response.Code,
			response.Body.String(),
		)
	}
	if attachments.uploadCalls != 0 {
		t.Fatalf(
			"超限文件不应调用上传服务，调用次数 = %d",
			attachments.uploadCalls,
		)
	}
}
