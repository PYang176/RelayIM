package messaging

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/PYang176/RelayIM/internal/platform/rabbitmq"
	"github.com/PYang176/RelayIM/internal/ws"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// TestMessageEventProcessorIntegration 覆盖消息事件的幂等处理和实时投递事实
func TestMessageEventProcessorIntegration(t *testing.T) {
	dsn := os.Getenv("RELAYIM_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("未设置 RELAYIM_TEST_MYSQL_DSN，跳过 MySQL 集成测试")
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
	userC := baseID + 2
	conversationID := baseID + 3
	messageID := baseID + 4
	eventID := baseID + 5
	recallEventID := baseID + 6
	invitationID := baseID + 7
	invitationEventID := baseID + 8
	invitationCancelEventID := baseID + 9
	t.Cleanup(func() {
		statements := []struct {
			query string
			args  []any
		}{
			{
				"DELETE FROM processed_events WHERE event_id IN (?, ?, ?)",
				[]any{eventID, recallEventID, invitationEventID},
			},
			{"DELETE FROM processed_events WHERE event_id = ?", []any{invitationCancelEventID}},
			{"DELETE FROM group_invitations WHERE id = ?", []any{invitationID}},
			{"DELETE FROM messages WHERE id = ?", []any{messageID}},
			{"DELETE FROM conversation_members WHERE conversation_id = ?", []any{conversationID}},
			{"DELETE FROM conversations WHERE id = ?", []any{conversationID}},
			{"DELETE FROM users WHERE id IN (?, ?, ?)", []any{userA, userB, userC}},
		}
		for _, statement := range statements {
			if err := db.Exec(statement.query, statement.args...).Error; err != nil {
				t.Errorf("清理消息消费测试数据失败: %v", err)
			}
		}
	})

	for _, userID := range []uint64{userA, userB, userC} {
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
	if err := db.Exec(
		`INSERT INTO group_invitations
			(id, conversation_id, inviter_id, invitee_id, invite_remark,
			 status, created_at, expires_at)
		 VALUES (?, ?, ?, ?, ?, 1, ?, ?)`,
		invitationID,
		conversationID,
		userA,
		userC,
		"邀请参与实时通知测试",
		time.Now().UTC(),
		time.Now().UTC().Add(24*time.Hour),
	).Error; err != nil {
		t.Fatalf("创建群邀请消费测试数据失败: %v", err)
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

	delivery := &stubEventDeliveryRouter{}
	processor := NewEventProcessor(db, delivery)
	if err := processor.Process(context.Background(), envelope); err != nil {
		t.Fatalf("首次处理消息事件失败: %v", err)
	}
	// 重复投递必须返回成功，且不能产生第二份幂等记录。
	if err := processor.Process(context.Background(), envelope); err != nil {
		t.Fatalf("重复处理消息事件失败: %v", err)
	}

	var processedCount int64
	if err := db.Model(&processedEvent{}).
		Where("event_id = ? AND consumer_name = ?", eventID, businessEventsConsumerName).
		Count(&processedCount).Error; err != nil {
		t.Fatalf("查询消费幂等记录失败: %v", err)
	}
	if processedCount != 1 {
		t.Fatalf("消费幂等记录数量 = %d，期望 1", processedCount)
	}
	if len(delivery.messages) != 1 {
		t.Fatalf("实时投递次数 = %d，期望 1", len(delivery.messages))
	}
	if len(delivery.userIDs) != 2 ||
		!containsUserID(delivery.userIDs, userA) ||
		!containsUserID(delivery.userIDs, userB) {
		t.Fatalf("实时接收者 = %v，期望会话全部成员", delivery.userIDs)
	}
	var pushed ws.WsMessage
	if err := json.Unmarshal(delivery.messages[0], &pushed); err != nil {
		t.Fatalf("解析实时消息失败: %v", err)
	}
	if pushed.MessageID != fmt.Sprintf("%d", messageID) ||
		pushed.ConversationID != fmt.Sprintf("%d", conversationID) ||
		pushed.SenderID != fmt.Sprintf("%d", userA) ||
		pushed.Seq != 1 ||
		pushed.Content != "消费测试消息" {
		t.Fatalf("实时消息 = %+v", pushed)
	}

	recalledAt := time.Now().UTC().Truncate(time.Millisecond)
	if err := db.Model(&message{}).
		Where("id = ?", messageID).
		Updates(map[string]any{
			"recalled_at": recalledAt,
			"recalled_by": userA,
		}).Error; err != nil {
		t.Fatalf("准备撤回状态失败: %v", err)
	}
	recallPayload, err := json.Marshal(messageRecalledPayload{
		MessageID:      fmt.Sprintf("%d", messageID),
		ConversationID: fmt.Sprintf("%d", conversationID),
		SenderID:       fmt.Sprintf("%d", userA),
		RecalledBy:     fmt.Sprintf("%d", userA),
		Seq:            1,
		RecalledAt:     recalledAt,
	})
	if err != nil {
		t.Fatalf("序列化撤回测试载荷失败: %v", err)
	}
	recallEnvelope := rabbitmq.EventEnvelope{
		EventID:     fmt.Sprintf("%d", recallEventID),
		EventType:   rabbitmq.MessageRecalledRoutingKey,
		OccurredAt:  recalledAt,
		AggregateID: fmt.Sprintf("%d", conversationID),
		Payload:     recallPayload,
	}
	if err := processor.Process(context.Background(), recallEnvelope); err != nil {
		t.Fatalf("处理消息撤回事件失败: %v", err)
	}
	if len(delivery.messages) != 2 {
		t.Fatalf("实时投递次数 = %d，期望创建和撤回共 2 次", len(delivery.messages))
	}
	if len(delivery.userIDs) != 2 ||
		delivery.userIDs[0] != userA ||
		delivery.userIDs[1] != userB {
		t.Fatalf("撤回实时接收者 = %v，期望群内两名成员", delivery.userIDs)
	}
	var recalledPush ws.WsMessage
	if err := json.Unmarshal(delivery.messages[1], &recalledPush); err != nil {
		t.Fatalf("解析实时撤回消息失败: %v", err)
	}
	if recalledPush.Action != ws.ActionMessageRecalled ||
		recalledPush.MessageID != fmt.Sprintf("%d", messageID) ||
		recalledPush.RecalledBy != fmt.Sprintf("%d", userA) ||
		recalledPush.RecalledAt == "" {
		t.Fatalf("实时撤回消息 = %+v", recalledPush)
	}

	invitationPayload, err := json.Marshal(groupInvitationEventPayload{
		InvitationID:   fmt.Sprintf("%d", invitationID),
		ConversationID: fmt.Sprintf("%d", conversationID),
		InviteeID:      fmt.Sprintf("%d", userC),
	})
	if err != nil {
		t.Fatalf("序列化群邀请事件载荷失败: %v", err)
	}
	invitationEnvelope := rabbitmq.EventEnvelope{
		EventID:     fmt.Sprintf("%d", invitationEventID),
		EventType:   rabbitmq.GroupInvitationCreatedRoutingKey,
		OccurredAt:  time.Now().UTC(),
		AggregateID: fmt.Sprintf("%d", conversationID),
		Payload:     invitationPayload,
	}
	if err := processor.Process(
		context.Background(),
		invitationEnvelope,
	); err != nil {
		t.Fatalf("处理群邀请实时事件失败: %v", err)
	}
	if len(delivery.userIDs) != 1 || delivery.userIDs[0] != userC {
		t.Fatalf("群邀请接收者 = %v，期望被邀请人", delivery.userIDs)
	}
	var invitationPush ws.GroupInvitationMessage
	if err := json.Unmarshal(delivery.messages[2], &invitationPush); err != nil {
		t.Fatalf("解析群邀请实时帧失败: %v", err)
	}
	if invitationPush.Action != ws.ActionGroupInvitation ||
		invitationPush.InvitationID != fmt.Sprintf("%d", invitationID) ||
		invitationPush.InviteRemark != "邀请参与实时通知测试" {
		t.Fatalf("群邀请实时帧 = %+v", invitationPush)
	}

	if err := db.Table("group_invitations").
		Where("id = ?", invitationID).
		Updates(map[string]any{
			"status":       5,
			"responded_at": time.Now().UTC(),
		}).Error; err != nil {
		t.Fatalf("准备群邀请取消状态失败: %v", err)
	}
	cancelEnvelope := invitationEnvelope
	cancelEnvelope.EventID = fmt.Sprintf("%d", invitationCancelEventID)
	cancelEnvelope.EventType = rabbitmq.GroupInvitationCancelledRoutingKey
	if err := processor.Process(context.Background(), cancelEnvelope); err != nil {
		t.Fatalf("处理群邀请取消事件失败: %v", err)
	}
	var cancelPush ws.GroupInvitationMessage
	if err := json.Unmarshal(delivery.messages[3], &cancelPush); err != nil {
		t.Fatalf("解析群邀请取消实时帧失败: %v", err)
	}
	if cancelPush.Action != ws.ActionGroupInvitationCancelled ||
		cancelPush.InvitationID != fmt.Sprintf("%d", invitationID) {
		t.Fatalf("群邀请取消实时帧 = %+v", cancelPush)
	}
}

// containsUserID 判断目标用户 ID 是否存在于给定集合
func containsUserID(userIDs []uint64, target uint64) bool {
	for _, userID := range userIDs {
		if userID == target {
			return true
		}
	}
	return false
}

type stubEventDeliveryRouter struct {
	userIDs  []uint64
	messages [][]byte
}

// Route 计算目标并完成路由
func (s *stubEventDeliveryRouter) Route(
	_ context.Context,
	userIDs []uint64,
	message []byte,
) error {
	s.userIDs = append([]uint64(nil), userIDs...)
	s.messages = append(s.messages, append([]byte(nil), message...))
	return nil
}
