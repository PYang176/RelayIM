package messaging

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

// claimOutbox 在同一事务内领取待发布或租约已过期的 Outbox 事件。
func (r *Repository) claimOutbox(
	ctx context.Context,
	workerID string,
	now time.Time,
	lease time.Duration,
	limit int,
) ([]outboxEvent, error) {
	var events []outboxEvent
	// 空闲轮询是正常路径，不按 Info 级别重复输出无结果 SQL。
	database := r.db.WithContext(ctx).Session(&gorm.Session{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	err := database.Transaction(func(tx *gorm.DB) error {
		var eventIDs []uint64
		if err := tx.Model(&outboxEvent{}).
			Select("id").
			Where(
				"(status = ? AND available_at <= ?) OR (status = ? AND locked_at < ?)",
				outboxStatusPending,
				now,
				outboxStatusProcessing,
				now.Add(-lease),
			).
			Order("id ASC").
			Limit(limit).
			Clauses(clause.Locking{
				Strength: "UPDATE",
				Options:  "SKIP LOCKED",
			}).
			Find(&eventIDs).Error; err != nil {
			return fmt.Errorf("查询待发布 Outbox 事件: %w", err)
		}
		if len(eventIDs) == 0 {
			return nil
		}

		result := tx.Model(&outboxEvent{}).
			Where("id IN ?", eventIDs).
			Updates(map[string]any{
				"status":        outboxStatusProcessing,
				"locked_by":     workerID,
				"locked_at":     now,
				"attempt_count": gorm.Expr("attempt_count + 1"),
			})
		if result.Error != nil {
			return fmt.Errorf("领取 Outbox 事件: %w", result.Error)
		}
		if result.RowsAffected != int64(len(eventIDs)) {
			return fmt.Errorf(
				"领取 Outbox 事件数量不一致: %d/%d",
				result.RowsAffected,
				len(eventIDs),
			)
		}

		if err := tx.Where("id IN ?", eventIDs).
			Order("id ASC").
			Find(&events).Error; err != nil {
			return fmt.Errorf("读取已领取 Outbox 事件: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return events, nil
}

// markOutboxPublished 在收到 Publisher Confirm 后标记事件发布成功。
func (r *Repository) markOutboxPublished(
	ctx context.Context,
	eventID uint64,
	workerID string,
	publishedAt time.Time,
) error {
	return r.updateClaimedOutbox(
		ctx,
		eventID,
		workerID,
		map[string]any{
			"status":       outboxStatusPublished,
			"published_at": publishedAt,
			"locked_by":    nil,
			"locked_at":    nil,
			"last_error":   nil,
		},
	)
}

// retryOutbox 把可重试发布失败放回待发布状态，并延迟下一次领取时间。
func (r *Repository) retryOutbox(
	ctx context.Context,
	eventID uint64,
	workerID string,
	availableAt time.Time,
	lastError string,
) error {
	return r.updateClaimedOutbox(
		ctx,
		eventID,
		workerID,
		map[string]any{
			"status":       outboxStatusPending,
			"available_at": availableAt,
			"locked_by":    nil,
			"locked_at":    nil,
			"last_error":   lastError,
		},
	)
}

// markOutboxDead 将超过最大发布次数的异常事件留在数据库供人工排查。
func (r *Repository) markOutboxDead(
	ctx context.Context,
	eventID uint64,
	workerID string,
	lastError string,
) error {
	return r.updateClaimedOutbox(
		ctx,
		eventID,
		workerID,
		map[string]any{
			"status":     outboxStatusDead,
			"locked_by":  nil,
			"locked_at":  nil,
			"last_error": lastError,
		},
	)
}

func (r *Repository) updateClaimedOutbox(
	ctx context.Context,
	eventID uint64,
	workerID string,
	updates map[string]any,
) error {
	result := r.db.WithContext(ctx).
		Model(&outboxEvent{}).
		Where(
			"id = ? AND status = ? AND locked_by = ?",
			eventID,
			outboxStatusProcessing,
			workerID,
		).
		Updates(updates)
	if result.Error != nil {
		return fmt.Errorf("更新 Outbox 事件 %d: %w", eventID, result.Error)
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("%w: event_id=%d", ErrOutboxLeaseLost, eventID)
	}
	return nil
}
