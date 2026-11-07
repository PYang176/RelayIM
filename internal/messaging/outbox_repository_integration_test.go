package messaging

import (
	"context"
	"os"
	"testing"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestOutboxRepositoryIntegration(t *testing.T) {
	dsn := os.Getenv("GINCHAT_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("未设置 GINCHAT_TEST_MYSQL_DSN，跳过 MySQL 集成测试")
	}

	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("连接测试数据库失败: %v", err)
	}

	eventID := uint64(time.Now().UnixNano())
	event := &outboxEvent{
		EventID:       eventID,
		AggregateType: "conversation",
		AggregateID:   eventID,
		EventType:     "message.created.v1",
		Payload:       `{"message_id":"1"}`,
		Status:        outboxStatusPending,
		AvailableAt:   time.Now().UTC().Add(-time.Second),
		CreatedAt:     time.Now().UTC(),
	}
	if err := db.Create(event).Error; err != nil {
		t.Fatalf("创建 Outbox 测试事件失败: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Where("event_id = ?", eventID).Delete(&outboxEvent{}).Error; err != nil {
			t.Errorf("清理 Outbox 测试事件失败: %v", err)
		}
	})

	repository := NewRepository(db)
	events, err := repository.claimOutbox(
		context.Background(),
		"integration-worker",
		time.Now().UTC(),
		30*time.Second,
		10,
	)
	if err != nil {
		t.Fatalf("领取 Outbox 事件失败: %v", err)
	}
	if len(events) != 1 || events[0].EventID != eventID {
		t.Fatalf("领取事件 = %+v，期望 event_id=%d", events, eventID)
	}
	if events[0].AttemptCount != 1 {
		t.Fatalf("发布尝试次数 = %d，期望 1", events[0].AttemptCount)
	}

	if err := repository.retryOutbox(
		context.Background(),
		event.ID,
		"integration-worker",
		time.Now().UTC().Add(time.Minute),
		"broker unavailable",
	); err != nil {
		t.Fatalf("安排 Outbox 重试失败: %v", err)
	}

	events, err = repository.claimOutbox(
		context.Background(),
		"integration-worker",
		time.Now().UTC().Add(2*time.Minute),
		30*time.Second,
		10,
	)
	if err != nil {
		t.Fatalf("重新领取 Outbox 事件失败: %v", err)
	}
	if len(events) != 1 || events[0].AttemptCount != 2 {
		t.Fatalf("重新领取事件 = %+v，期望尝试次数 2", events)
	}

	if err := repository.markOutboxPublished(
		context.Background(),
		event.ID,
		"integration-worker",
		time.Now().UTC(),
	); err != nil {
		t.Fatalf("标记 Outbox 发布成功失败: %v", err)
	}

	var published outboxEvent
	if err := db.Where("event_id = ?", eventID).First(&published).Error; err != nil {
		t.Fatalf("读取已发布 Outbox 事件失败: %v", err)
	}
	if published.Status != outboxStatusPublished || published.PublishedAt == nil {
		t.Fatalf("已发布事件 = %+v，期望 status=2 且 published_at 非空", published)
	}
}
