// Package objectstore 封装 SeaweedFS 的 S3 兼容对象存储。
package objectstore

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/PYang176/RelayIM/internal/config"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// Client 负责附件对象的上传、读取和删除。
type Client struct {
	client        *minio.Client
	presignClient *minio.Client
	bucket        string
}

// NewClient 创建 S3 客户端并确保附件 Bucket 存在。
func NewClient(
	ctx context.Context,
	cfg config.ObjectStorageConfig,
) (*Client, error) {
	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds: credentials.NewStaticV4(
			cfg.AccessKey,
			cfg.SecretKey,
			"",
		),
		Secure: cfg.UseSSL,
		Region: cfg.Region,
	})
	if err != nil {
		return nil, fmt.Errorf("创建 S3 客户端: %w", err)
	}
	presignClient, err := newPresignClient(cfg)
	if err != nil {
		return nil, err
	}

	exists, err := client.BucketExists(ctx, cfg.Bucket)
	if err != nil {
		return nil, fmt.Errorf("检查 S3 Bucket: %w", err)
	}
	if !exists {
		if err := client.MakeBucket(
			ctx,
			cfg.Bucket,
			minio.MakeBucketOptions{Region: cfg.Region},
		); err != nil {
			return nil, fmt.Errorf("创建 S3 Bucket: %w", err)
		}
	}
	return &Client{
		client:        client,
		presignClient: presignClient,
		bucket:        cfg.Bucket,
	}, nil
}

// newPresignClient 创建仅用于生成浏览器可访问预签名地址的 S3 客户端。
func newPresignClient(cfg config.ObjectStorageConfig) (*minio.Client, error) {
	host, secure, err := parsePublicEndpoint(cfg.PublicEndpoint)
	if err != nil {
		return nil, err
	}
	if host == "" {
		return nil, nil
	}
	client, err := minio.New(host, &minio.Options{
		Creds: credentials.NewStaticV4(
			cfg.AccessKey,
			cfg.SecretKey,
			"",
		),
		Secure: secure,
		Region: cfg.Region,
	})
	if err != nil {
		return nil, fmt.Errorf("创建 S3 公网预签名客户端: %w", err)
	}
	return client, nil
}

// parsePublicEndpoint 校验浏览器访问 S3 对象使用的完整地址。
func parsePublicEndpoint(raw string) (string, bool, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false, nil
	}
	endpoint, err := url.Parse(raw)
	if err != nil || endpoint.Host == "" ||
		(endpoint.Scheme != "http" && endpoint.Scheme != "https") ||
		(endpoint.Path != "" && endpoint.Path != "/") ||
		endpoint.RawQuery != "" || endpoint.Fragment != "" ||
		endpoint.User != nil {
		return "", false, fmt.Errorf("S3 公网地址必须是合法的 HTTP(S) 源地址")
	}
	return endpoint.Host, endpoint.Scheme == "https", nil
}

// PutObject 上传附件对象。
func (c *Client) PutObject(
	ctx context.Context,
	objectKey string,
	reader io.Reader,
	size int64,
	contentType string,
) error {
	_, err := c.client.PutObject(
		ctx,
		c.bucket,
		objectKey,
		reader,
		size,
		minio.PutObjectOptions{ContentType: contentType},
	)
	if err != nil {
		return fmt.Errorf("上传 S3 对象: %w", err)
	}
	return nil
}

// OpenObject 读取附件对象并返回内容类型和大小。
func (c *Client) OpenObject(
	ctx context.Context,
	objectKey string,
) (io.ReadCloser, string, int64, error) {
	object, err := c.client.GetObject(ctx, c.bucket, objectKey, minio.GetObjectOptions{})
	if err != nil {
		return nil, "", 0, fmt.Errorf("读取 S3 对象: %w", err)
	}
	info, err := object.Stat()
	if err != nil {
		_ = object.Close()
		return nil, "", 0, fmt.Errorf("查询 S3 对象详情: %w", err)
	}
	return object, info.ContentType, info.Size, nil
}

// RemoveObject 删除附件对象，供上传失败或后续清理使用。
func (c *Client) RemoveObject(ctx context.Context, objectKey string) error {
	if err := c.client.RemoveObject(
		ctx,
		c.bucket,
		objectKey,
		minio.RemoveObjectOptions{},
	); err != nil {
		return fmt.Errorf("删除 S3 对象: %w", err)
	}
	return nil
}

// StatObject 返回对象大小和内容类型，用于上传后确认。
func (c *Client) StatObject(
	ctx context.Context,
	objectKey string,
) (string, int64, error) {
	info, err := c.client.StatObject(
		ctx,
		c.bucket,
		objectKey,
		minio.StatObjectOptions{},
	)
	if err != nil {
		return "", 0, fmt.Errorf("查询 S3 对象: %w", err)
	}
	return info.ContentType, info.Size, nil
}

// PresignedGetURL 生成短时附件下载地址。
func (c *Client) PresignedGetURL(
	ctx context.Context,
	objectKey string,
	expiry time.Duration,
) (string, error) {
	signClient := c.client
	if c.presignClient != nil {
		signClient = c.presignClient
	}
	value, err := signClient.PresignedGetObject(
		ctx,
		c.bucket,
		objectKey,
		expiry,
		url.Values{},
	)
	if err != nil {
		return "", fmt.Errorf("生成 S3 预签名地址: %w", err)
	}
	return value.String(), nil
}
