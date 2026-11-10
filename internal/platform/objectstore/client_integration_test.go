package objectstore

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"testing"
	"time"

	"ginChatProject/internal/config"
)

// TestObjectStorageIntegration 覆盖 SeaweedFS 对象写入、读取和删除
func TestObjectStorageIntegration(t *testing.T) {
	endpoint := os.Getenv("GINCHAT_TEST_S3_ENDPOINT")
	accessKey := os.Getenv("GINCHAT_TEST_S3_ACCESS_KEY")
	secretKey := os.Getenv("GINCHAT_TEST_S3_SECRET_KEY")
	if endpoint == "" || accessKey == "" || secretKey == "" {
		t.Skip("未设置测试 S3 配置，跳过对象存储集成测试")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	client, err := NewClient(ctx, config.ObjectStorageConfig{
		Endpoint:  endpoint,
		AccessKey: accessKey,
		SecretKey: secretKey,
		Bucket:    "ginchat-test",
		Region:    "us-east-1",
	})
	if err != nil {
		t.Fatalf("创建对象存储客户端失败: %v", err)
	}

	key := fmt.Sprintf("tests/%d.txt", time.Now().UnixNano())
	body := []byte("ginchat object storage integration")
	if err := client.PutObject(
		ctx,
		key,
		bytes.NewReader(body),
		int64(len(body)),
		"text/plain",
	); err != nil {
		t.Fatalf("上传测试对象失败: %v", err)
	}
	t.Cleanup(func() {
		if err := client.RemoveObject(context.Background(), key); err != nil {
			t.Errorf("清理测试对象失败: %v", err)
		}
	})

	reader, contentType, size, err := client.OpenObject(ctx, key)
	if err != nil {
		t.Fatalf("读取测试对象失败: %v", err)
	}
	defer reader.Close()
	content, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("读取测试对象内容失败: %v", err)
	}
	if !bytes.Equal(content, body) ||
		contentType != "text/plain" ||
		size != int64(len(body)) {
		t.Fatalf("对象信息 = type:%s size:%d body:%q", contentType, size, content)
	}
}
