package messaging

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"sync"
	"testing"

	"ginChatProject/internal/platform/snowflake"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestRepositorySendTextIntegration(t *testing.T) {
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
	if err := snowflake.Init(1); err != nil {
		t.Fatalf("初始化 Snowflake 失败: %v", err)
	}

	ownerID := uint64(snowflake.Generate())
	memberID := uint64(snowflake.Generate())
	outsiderID := uint64(snowflake.Generate())
	conversationID := uint64(snowflake.Generate())
	userIDs := []uint64{ownerID, memberID, outsiderID}

	t.Cleanup(func() {
		cleanupStatements := []struct {
			query string
			args  []any
		}{
			{"DELETE FROM outbox_events WHERE aggregate_id = ?", []any{conversationID}},
			{"DELETE FROM messages WHERE conversation_id = ?", []any{conversationID}},
			{"DELETE FROM conversation_members WHERE conversation_id = ?", []any{conversationID}},
			{"DELETE FROM conversations WHERE id = ?", []any{conversationID}},
			{"DELETE FROM users WHERE id IN (?, ?, ?)", []any{ownerID, memberID, outsiderID}},
		}
		for _, statement := range cleanupStatements {
			if err := db.Exec(statement.query, statement.args...).Error; err != nil {
				t.Errorf("清理测试数据失败: %v", err)
			}
		}
	})

	for _, userID := range userIDs {
		if err := db.Exec(
			"INSERT INTO users (id, username, password_hash, status) VALUES (?, ?, ?, 1)",
			userID,
			fmt.Sprintf("mt_%d", userID),
			"not-used",
		).Error; err != nil {
			t.Fatalf("创建测试用户失败: %v", err)
		}
	}

	if err := db.Exec(
		`INSERT INTO conversations
			(id, conversation_type, owner_id, name, status, last_seq)
		 VALUES (?, 2, ?, ?, 1, 0)`,
		conversationID,
		ownerID,
		"消息事务测试群",
	).Error; err != nil {
		t.Fatalf("创建测试会话失败: %v", err)
	}
	if err := db.Exec(
		`INSERT INTO conversation_members
			(conversation_id, user_id, role)
		 VALUES (?, ?, 3), (?, ?, 1)`,
		conversationID,
		ownerID,
		conversationID,
		memberID,
	).Error; err != nil {
		t.Fatalf("创建测试会话成员失败: %v", err)
	}

	service := NewService(NewRepository(db))
	first, err := service.SendText(context.Background(), SendTextCommand{
		ConversationID:  conversationID,
		SenderID:        ownerID,
		ClientMessageID: "client-message-1",
		Content:         "第一条消息",
	})
	if err != nil {
		t.Fatalf("发送第一条消息失败: %v", err)
	}
	if first.Seq != 1 || first.Duplicate {
		t.Fatalf("首条消息结果 = %+v，期望 seq=1 且非重复", first)
	}

	duplicate, err := service.SendText(context.Background(), SendTextCommand{
		ConversationID:  conversationID,
		SenderID:        ownerID,
		ClientMessageID: "client-message-1",
		Content:         "重复请求",
	})
	if err != nil {
		t.Fatalf("重复发送消息失败: %v", err)
	}
	if !duplicate.Duplicate || duplicate.MessageID != first.MessageID {
		t.Fatalf("重复请求结果 = %+v，期望返回原消息", duplicate)
	}

	_, err = service.SendText(context.Background(), SendTextCommand{
		ConversationID:  conversationID,
		SenderID:        outsiderID,
		ClientMessageID: "client-message-outsider",
		Content:         "非法消息",
	})
	if !errors.Is(err, ErrNotMember) {
		t.Fatalf("非成员发送错误 = %v，期望 ErrNotMember", err)
	}

	const concurrentMessages = 5
	results := make(chan SendTextResult, concurrentMessages)
	errs := make(chan error, concurrentMessages)
	var wg sync.WaitGroup
	for index := 0; index < concurrentMessages; index++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			result, sendErr := service.SendText(context.Background(), SendTextCommand{
				ConversationID:  conversationID,
				SenderID:        memberID,
				ClientMessageID: fmt.Sprintf("client-concurrent-%d", index),
				Content:         fmt.Sprintf("并发消息 %d", index),
			})
			if sendErr != nil {
				errs <- sendErr
				return
			}
			results <- result
		}(index)
	}
	wg.Wait()
	close(results)
	close(errs)

	for sendErr := range errs {
		t.Errorf("并发发送消息失败: %v", sendErr)
	}

	sequences := make([]uint64, 0, concurrentMessages)
	for result := range results {
		sequences = append(sequences, result.Seq)
	}
	sort.Slice(sequences, func(i, j int) bool { return sequences[i] < sequences[j] })
	for index, seq := range sequences {
		want := uint64(index + 2)
		if seq != want {
			t.Fatalf("并发消息序号 = %v，期望从 2 连续递增", sequences)
		}
	}

	var outboxCount int64
	if err := db.Model(&outboxEvent{}).
		Where("aggregate_id = ? AND event_type = ?", conversationID, "message.created.v1").
		Count(&outboxCount).Error; err != nil {
		t.Fatalf("查询 Outbox 数量失败: %v", err)
	}
	if want := int64(concurrentMessages + 1); outboxCount != want {
		t.Fatalf("Outbox 数量 = %d，期望 %d", outboxCount, want)
	}
}
