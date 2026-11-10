package messaging

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"ginChatProject/internal/config"
	"ginChatProject/internal/platform/observability"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type attachmentCleanupStore interface {
	claimOrphanAttachments(
		ctx context.Context,
		now time.Time,
		pendingTTL time.Duration,
		lease time.Duration,
		limit int,
	) ([]attachmentCleanupCandidate, error)
	markAttachmentCleanupFailed(
		ctx context.Context,
		attachmentID uint64,
	) error
	deleteCleanedAttachmentMetadata(
		ctx context.Context,
		attachmentID uint64,
	) error
}

type attachmentCleanupStorage interface {
	RemoveObject(ctx context.Context, objectKey string) error
}

// AttachmentCleanupMetrics 记录孤儿附件清理结果。
type AttachmentCleanupMetrics interface {
	RecordAttachmentCleanup(result string)
}

// AttachmentCleanupWorker 定时删除未绑定消息的过期附件。
type AttachmentCleanupWorker struct {
	store   attachmentCleanupStore
	storage attachmentCleanupStorage
	config  config.AttachmentCleanupConfig
	metrics AttachmentCleanupMetrics
	now     func() time.Time
}

// NewAttachmentCleanupWorker 创建孤儿附件清理任务。
func NewAttachmentCleanupWorker(
	store attachmentCleanupStore,
	storage attachmentCleanupStorage,
	cfg config.AttachmentCleanupConfig,
	metrics AttachmentCleanupMetrics,
) *AttachmentCleanupWorker {
	return &AttachmentCleanupWorker{
		store:   store,
		storage: storage,
		config:  cfg,
		metrics: metrics,
		now:     time.Now,
	}
}

// Run 立即执行一轮清理，之后按配置间隔持续运行。
func (w *AttachmentCleanupWorker) Run(ctx context.Context) {
	interval := time.Duration(w.config.IntervalSeconds) * time.Second
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			if _, err := w.CleanupOnce(ctx); err != nil {
				slog.Error("孤儿附件清理失败", "error", err)
			}
			timer.Reset(interval)
		}
	}
}

// CleanupOnce 执行一轮有上限的孤儿附件清理。
func (w *AttachmentCleanupWorker) CleanupOnce(
	ctx context.Context,
) (int, error) {
	ctx, span := observability.Tracer().Start(
		ctx,
		"attachment.cleanup",
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(attribute.Int(
			"ginchat.attachment.batch_size",
			w.config.BatchSize,
		)),
	)
	defer span.End()

	now := w.now().UTC()
	pendingTTL := time.Duration(w.config.PendingTTLHours) * time.Hour
	lease := time.Duration(w.config.ClaimLeaseSeconds) * time.Second
	candidates, err := w.store.claimOrphanAttachments(
		ctx,
		now,
		pendingTTL,
		lease,
		w.config.BatchSize,
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return 0, err
	}
	span.SetAttributes(attribute.Int(
		"ginchat.attachment.candidates",
		len(candidates),
	))

	deleted := 0
	for _, candidate := range candidates {
		if err := w.cleanupCandidate(ctx, candidate); err != nil {
			slog.Warn(
				"清理孤儿附件失败",
				"attachment_id", candidate.ID,
				"error", err,
			)
			markCtx, cancel := context.WithTimeout(
				context.WithoutCancel(ctx),
				5*time.Second,
			)
			if markErr := w.store.markAttachmentCleanupFailed(
				markCtx,
				candidate.ID,
			); markErr != nil {
				slog.Warn(
					"重置孤儿附件清理状态失败",
					"attachment_id", candidate.ID,
					"error", markErr,
				)
			}
			cancel()
			w.recordMetric("failed")
			continue
		}
		deleted++
		w.recordMetric("deleted")
	}
	span.SetAttributes(attribute.Int("ginchat.attachment.deleted", deleted))
	return deleted, nil
}

// cleanupCandidate 删除单个孤儿附件对象及其元数据
func (w *AttachmentCleanupWorker) cleanupCandidate(
	ctx context.Context,
	candidate attachmentCleanupCandidate,
) error {
	cleanupCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if err := w.storage.RemoveObject(
		cleanupCtx,
		candidate.ObjectKey,
	); err != nil {
		return fmt.Errorf("删除孤儿附件对象: %w", err)
	}
	if err := w.store.deleteCleanedAttachmentMetadata(
		cleanupCtx,
		candidate.ID,
	); err != nil {
		return fmt.Errorf("删除孤儿附件元数据: %w", err)
	}
	return nil
}

// recordMetric 记录指标
func (w *AttachmentCleanupWorker) recordMetric(result string) {
	if w.metrics != nil {
		w.metrics.RecordAttachmentCleanup(result)
	}
}
