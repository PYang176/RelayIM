package messaging

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"ginChatProject/internal/platform/rabbitmq"
	"ginChatProject/internal/ws"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestMessageCreatedProcessorIntegration(t *testing.T) {
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

	baseID := uint64(time.Now().UnixNano())
	userA := baseID
	userB := baseID + 1
	conversationID := baseID + 2
	messageID := baseID + 3
	eventID := baseID + 4
	t.Cleanup(func() {
		statements := []struct {
			query string
			args  []any
		}{
			{"DELETE FROM processed_events WHERE event_id = ?", []any{eventID}},
			{"DELETE FROM messages WHERE id = ?", []any{messageID}},
			{"DELETE FROM conversation_members WHERE conversation_id = ?", []any{conversationID}},
			{"DELETE FROM conversations WHERE id = ?", []any{conversationID}},
			{"DELETE FROM users WHERE id IN (?, ?)", []any{userA, userB}},
		}
		for _, statement := range statements {
			if err := db.Exec(statement.query, statement.args...).Error; err != nil {
				t.Errorf("清理消息消费测试数据失败: %v", err)
			}
		}
	})

	for _, userID := range []uint64{userA, userB} {
		if err := db.Exec(
			"INSERT INTO users (id, username, password_hash, status) VALUES (?, ?, ?, 1)",
			userID,
			fmt.Sprintf("consumer_%d", userID),
			"not-used",
		).Error; err != nil {
			t.Fatalf("创建消费测试用户失败: %v", err)
		}
	}
	if err := db.Exec(
		`INSERT INTO conversations
			(id, conversation_type, owner_id, name, status, last_seq, last_message_id)
		 VALUES (?, 2, ?, ?, 1, 1, ?)`,
		conversationID,
		userA,
		"消息消费测试群",
		messageID,
	).Error; err != nil {
		t.Fatalf("创建消费测试会话失败: %v", err)
	}
	if err := db.Exec(
		`INSERT INTO conversation_members
			(conversation_id, user_id, role)
		 VALUES (?, ?, 3), (?, ?, 1)`,
		conversationID,
		userA,
		conversationID,
		userB,
	).Error; err != nil {
		t.Fatalf("创建消费测试成员失败: %v", err)
	}
	if err := db.Exec(
		`INSERT INTO messages
			(id, conversation_id, seq, sender_id, client_msg_id, message_type, content)
		 VALUES (?, ?, 1, ?, ?, 1, ?)`,
		messageID,
		conversationID,
		userA,
		fmt.Sprintf("client-%d", messageID),
		"消费测试消息",
	).Error; err != nil {
		t.Fatalf("创建消费测试消息失败: %v", err)
	}

	payload, err := json.Marshal(messageCreatedPayload{
		MessageID:      fmt.Sprintf("%d", messageID),
		ConversationID: fmt.Sprintf("%d", conversationID),
		SenderID:       fmt.Sprintf("%d", userA),
		Seq:            1,
	})
	if err != nil {
		t.Fatalf("序列化测试载荷失败: %v", err)
	}
	envelope := rabbitmq.EventEnvelope{
		EventID:     fmt.Sprintf("%d", eventID),
		EventType:   rabbitmq.MessageCreatedRoutingKey,
		OccurredAt:  time.Now().UTC(),
		AggregateID: fmt.Sprintf("%d", conversationID),
		Payload:     payload,
	}

	realtime := &stubRealtimeSender{}
	processor := NewMessageCreatedProcessor(db, realtime)
	if err := processor.Process(context.Background(), envelope); err != nil {
		t.Fatalf("首次处理消息事件失败: %v", err)
	}
	// 重复投递必须返回成功，且不能产生第二份幂等记录。
	if err := processor.Process(context.Background(), envelope); err != nil {
		t.Fatalf("重复处理消息事件失败: %v", err)
	}

	var processedCount int64
	if err := db.Model(&processedEvent{}).
		Where("event_id = ? AND consumer_name = ?", eventID, messageCreatedConsumerName).
		Count(&processedCount).Error; err != nil {
		t.Fatalf("查询消费幂等记录失败: %v", err)
	}
	if processedCount != 1 {
		t.Fatalf("消费幂等记录数量 = %d，期望 1", processedCount)
	}
	if len(realtime.messages) != 1 {
		t.Fatalf("实时投递次数 = %d，期望 1", len(realtime.messages))
	}
	if len(realtime.userIDs) != 1 || realtime.userIDs[0] != userB {
		t.Fatalf("实时接收者 = %v，期望用户 %d", realtime.userIDs, userB)
	}
	var pushed ws.WsMessage
	if err := json.Unmarshal(realtime.messages[0], &pushed); err != nil {
		t.Fatalf("解析实时消息失败: %v", err)
	}
	if pushed.MessageID != fmt.Sprintf("%d", messageID) ||
		pushed.ConversationID != fmt.Sprintf("%d", conversationID) ||
		pushed.SenderID != fmt.Sprintf("%d", userA) ||
		pushed.Seq != 1 ||
		pushed.Content != "消费测试消息" {
		t.Fatalf("实时消息 = %+v", pushed)
	}
}

type stubRealtimeSender struct {
	userIDs  []uint64
	messages [][]byte
}

func (s *stubRealtimeSender) SendToUsers(
	userIDs []uint64,
	message []byte,
) int {
	s.userIDs = append([]uint64(nil), userIDs...)
	s.messages = append(s.messages, append([]byte(nil), message...))
	return len(userIDs)
}
