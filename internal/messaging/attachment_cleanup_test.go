package messaging

import (
	"context"
	"errors"
	"testing"
	"time"

	"ginChatProject/internal/config"
)

type stubCleanupStore struct {
	candidates []attachmentCleanupCandidate
	deletedIDs []uint64
	failedIDs  []uint64
	claimErr   error
}

func (s *stubCleanupStore) claimOrphanAttachments(
	context.Context,
	time.Time,
	time.Duration,
	time.Duration,
	int,
) ([]attachmentCleanupCandidate, error) {
	return s.candidates, s.claimErr
}

func (s *stubCleanupStore) markAttachmentCleanupFailed(
	_ context.Context,
	attachmentID uint64,
) error {
	s.failedIDs = append(s.failedIDs, attachmentID)
	return nil
}

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

func (s *stubCleanupMetrics) RecordAttachmentCleanup(result string) {
	s.results = append(s.results, result)
}

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
