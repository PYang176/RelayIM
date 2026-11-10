package messaging

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/PYang176/RelayIM/internal/config"
)

type stubCleanupStore struct {
	candidates []attachmentCleanupCandidate
	deletedIDs []uint64
	failedIDs  []uint64
	claimErr   error
}

// claimOrphanAttachments 领取超过 TTL 且未绑定消息的孤儿附件
func (s *stubCleanupStore) claimOrphanAttachments(
	context.Context,
	time.Time,
	time.Duration,
	time.Duration,
	int,
) ([]attachmentCleanupCandidate, error) {
	return s.candidates, s.claimErr
}

// markAttachmentCleanupFailed 把清理失败附件放回可重试状态
func (s *stubCleanupStore) markAttachmentCleanupFailed(
	_ context.Context,
	attachmentID uint64,
) error {
	s.failedIDs = append(s.failedIDs, attachmentID)
	return nil
}

// deleteCleanedAttachmentMetadata 删除Cleaned附件Metadata并遵循关联约束
func (s *stubCleanupStore) deleteCleanedAttachmentMetadata(
	_ context.Context,
	attachmentID uint64,
) error {
	s.deletedIDs = append(s.deletedIDs, attachmentID)
	return nil
}

type stubCleanupStorage struct {
	failKey string
	removed []string
}

// RemoveObject 移除对象并同步清理关联状态
func (s *stubCleanupStorage) RemoveObject(
	_ context.Context,
	objectKey string,
) error {
	s.removed = append(s.removed, objectKey)
	if objectKey == s.failKey {
		return errors.New("对象存储不可用")
	}
	return nil
}

type stubCleanupMetrics struct {
	results []string
}

// RecordAttachmentCleanup 记录附件清理
func (s *stubCleanupMetrics) RecordAttachmentCleanup(result string) {
	s.results = append(s.results, result)
}

// TestAttachmentCleanupDeletesAndRetries 覆盖孤儿附件成功删除，以及对象存储失败后回到可重试状态
func TestAttachmentCleanupDeletesAndRetries(t *testing.T) {
	store := &stubCleanupStore{
		candidates: []attachmentCleanupCandidate{
			{ID: 1, ObjectKey: "users/1/a"},
			{ID: 2, ObjectKey: "users/1/b"},
		},
	}
	storage := &stubCleanupStorage{failKey: "users/1/b"}
	metrics := &stubCleanupMetrics{}
	worker := NewAttachmentCleanupWorker(
		store,
		storage,
		config.AttachmentCleanupConfig{
			PendingTTLHours:   24,
			ClaimLeaseSeconds: 600,
			BatchSize:         100,
		},
		metrics,
	)
	worker.now = func() time.Time {
		return time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	}

	deleted, err := worker.CleanupOnce(context.Background())
	if err != nil {
		t.Fatalf("CleanupOnce() 错误: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("删除数量 = %d，期望 1", deleted)
	}
	if len(store.deletedIDs) != 1 || store.deletedIDs[0] != 1 {
		t.Fatalf("删除元数据 ID = %v", store.deletedIDs)
	}
	if len(store.failedIDs) != 1 || store.failedIDs[0] != 2 {
		t.Fatalf("失败重试 ID = %v", store.failedIDs)
	}
	if len(storage.removed) != 2 ||
		len(metrics.results) != 2 ||
		metrics.results[0] != "deleted" ||
		metrics.results[1] != "failed" {
		t.Fatalf(
			"清理结果 removed=%v metrics=%v",
			storage.removed,
			metrics.results,
		)
	}
}
