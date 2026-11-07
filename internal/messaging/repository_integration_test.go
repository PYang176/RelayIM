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
	// 测试会真实并发调用 Snowflake，不能在多个测试二进制之间共享同一节点。
	if err := snowflake.Init(1); err != nil {
		t.Fatalf("初始化 Snowflake 失败: %v", err)
	}

	ownerID := uint64(snowflake.Generate())
	memberID := uint64(snowflake.Generate())
	outsiderID := uint64(snowflake.Generate())
	conversationID := uint64(snowflake.Generate())
	memberDeviceID := uint64(snowflake.Generate())
	attachmentID := uint64(snowflake.Generate())
	userIDs := []uint64{ownerID, memberID, outsiderID}

	// 先注册清理函数，保证后续任何步骤失败时都不会把测试数据留在本地库。
	t.Cleanup(func() {
		cleanupStatements := []struct {
			query string
			args  []any
		}{
			{"DELETE FROM message_deliveries WHERE device_id = ?", []any{memberDeviceID}},
			{"DELETE FROM outbox_events WHERE aggregate_id = ?", []any{conversationID}},
			{"DELETE FROM messages WHERE conversation_id = ?", []any{conversationID}},
			{"DELETE FROM attachments WHERE id = ?", []any{attachmentID}},
			{"DELETE FROM conversation_members WHERE conversation_id = ?", []any{conversationID}},
			{"DELETE FROM conversations WHERE id = ?", []any{conversationID}},
			{"DELETE FROM devices WHERE id = ?", []any{memberDeviceID}},
			{"DELETE FROM users WHERE id IN (?, ?, ?)", []any{ownerID, memberID, outsiderID}},
		}
		for _, statement := range cleanupStatements {
			if err := db.Exec(statement.query, statement.args...).Error; err != nil {
				t.Errorf("清理测试数据失败: %v", err)
			}
		}
	})

	// 创建三个用户：群主、普通成员和用于验证权限拒绝的群外用户。
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
		`INSERT INTO devices
			(id, user_id, platform, device_name, last_seen_at)
		 VALUES (?, ?, 1, ?, CURRENT_TIMESTAMP(3))`,
		memberDeviceID,
		memberID,
		"消息回执测试设备",
	).Error; err != nil {
		t.Fatalf("创建测试设备失败: %v", err)
	}
	if err := db.Exec(
		`INSERT INTO attachments
			(id, owner_id, object_key, original_name, mime_type, size_bytes, status, finalized_at)
		 VALUES (?, ?, ?, ?, ?, ?, 2, CURRENT_TIMESTAMP(3))`,
		attachmentID,
		ownerID,
		fmt.Sprintf("tests/%d/image.png", attachmentID),
		"image.png",
		"image/png",
		1024,
	).Error; err != nil {
		t.Fatalf("创建测试附件失败: %v", err)
	}

	// 使用群会话测试，因为同一张 conversation_members 表也能代表发送权限。
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
	// 场景 1：正常发送首条消息，验证 seq 从 1 开始且首次请求不标记重复。
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

	// 场景 2：模拟客户端超时重试，必须返回原消息而不是再分配 seq。
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

	// 场景 3：群外用户即使知道 conversation_id，也不能绕过权限发送。
	_, err = service.SendText(context.Background(), SendTextCommand{
		ConversationID:  conversationID,
		SenderID:        outsiderID,
		ClientMessageID: "client-message-outsider",
		Content:         "非法消息",
	})
	if !errors.Is(err, ErrNotMember) {
		t.Fatalf("非成员发送错误 = %v，期望 ErrNotMember", err)
	}

	// 场景 4：同一会话并发发送。会话行锁应让序号仍保持连续，不发生重复。
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

	latestPage, err := service.ListMessages(context.Background(), ListMessagesCommand{
		ConversationID: conversationID,
		UserID:         memberID,
		Limit:          3,
	})
	if err != nil {
		t.Fatalf("查询最近历史消息失败: %v", err)
	}
	if got := messageSequences(latestPage.Messages); !equalSequences(got, []uint64{4, 5, 6}) {
		t.Fatalf("最近消息序号 = %v，期望 [4 5 6]", got)
	}
	if !latestPage.HasMore {
		t.Fatal("最近消息第一页应存在更早消息")
	}

	afterPage, err := service.ListMessages(context.Background(), ListMessagesCommand{
		ConversationID: conversationID,
		UserID:         memberID,
		AfterSeq:       2,
		Limit:          2,
	})
	if err != nil {
		t.Fatalf("after_seq 补拉失败: %v", err)
	}
	if got := messageSequences(afterPage.Messages); !equalSequences(got, []uint64{3, 4}) {
		t.Fatalf("after_seq 消息序号 = %v，期望 [3 4]", got)
	}
	if !afterPage.HasMore {
		t.Fatal("after_seq 第一页应存在更多消息")
	}

	beforePage, err := service.ListMessages(context.Background(), ListMessagesCommand{
		ConversationID: conversationID,
		UserID:         memberID,
		BeforeSeq:      5,
		Limit:          2,
	})
	if err != nil {
		t.Fatalf("before_seq 翻页失败: %v", err)
	}
	if got := messageSequences(beforePage.Messages); !equalSequences(got, []uint64{3, 4}) {
		t.Fatalf("before_seq 消息序号 = %v，期望 [3 4]", got)
	}

	if _, err := service.ListMessages(context.Background(), ListMessagesCommand{
		ConversationID: conversationID,
		UserID:         outsiderID,
		Limit:          3,
	}); !errors.Is(err, ErrNotMember) {
		t.Fatalf("非成员查询历史错误 = %v，期望 ErrNotMember", err)
	}

	if err := service.AcknowledgeMessage(
		context.Background(),
		memberID,
		memberDeviceID,
		first.MessageID,
	); err != nil {
		t.Fatalf("记录消息送达失败: %v", err)
	}
	// 重复 ACK 必须保持幂等。
	if err := service.AcknowledgeMessage(
		context.Background(),
		memberID,
		memberDeviceID,
		first.MessageID,
	); err != nil {
		t.Fatalf("重复记录消息送达失败: %v", err)
	}
	var deliveryCount int64
	if err := db.Model(&messageDelivery{}).
		Where("message_id = ? AND device_id = ?", first.MessageID, memberDeviceID).
		Count(&deliveryCount).Error; err != nil {
		t.Fatalf("查询送达回执失败: %v", err)
	}
	if deliveryCount != 1 {
		t.Fatalf("送达回执数量 = %d，期望 1", deliveryCount)
	}
	deliverySummary, err := service.GetDeliverySummary(
		context.Background(),
		ownerID,
		conversationID,
		first.MessageID,
	)
	if err != nil {
		t.Fatalf("查询送达汇总失败: %v", err)
	}
	if deliverySummary.DeliveredDevices != 1 ||
		deliverySummary.DeliveredUsers != 1 {
		t.Fatalf("送达汇总 = %+v，期望 1 台设备、1 个用户", deliverySummary)
	}

	if err := service.MarkConversationRead(
		context.Background(),
		memberID,
		conversationID,
		4,
	); err != nil {
		t.Fatalf("推进已读游标失败: %v", err)
	}
	if err := service.MarkConversationRead(
		context.Background(),
		memberID,
		conversationID,
		3,
	); err != nil {
		t.Fatalf("重复推进较小已读游标失败: %v", err)
	}
	readStates, err := service.ListReadStates(
		context.Background(),
		memberID,
		conversationID,
	)
	if err != nil {
		t.Fatalf("查询已读状态失败: %v", err)
	}
	if !hasReadState(readStates, memberID, 6) {
		t.Fatalf("成员已读状态 = %+v，期望发送者游标保持在 seq=6", readStates)
	}
	if err := service.MarkConversationRead(
		context.Background(),
		memberID,
		conversationID,
		7,
	); !errors.Is(err, ErrInvalidReadSequence) {
		t.Fatalf("超范围已读错误 = %v，期望 ErrInvalidReadSequence", err)
	}

	// 最后核对 Outbox 数量：1 条首消息加 5 条并发消息，每条都必须有事件。
	var outboxCount int64
	if err := db.Model(&outboxEvent{}).
		Where("aggregate_id = ? AND event_type = ?", conversationID, "message.created.v1").
		Count(&outboxCount).Error; err != nil {
		t.Fatalf("查询 Outbox 数量失败: %v", err)
	}
	if want := int64(concurrentMessages + 1); outboxCount != want {
		t.Fatalf("Outbox 数量 = %d，期望 %d", outboxCount, want)
	}

	mediaMessage, err := service.SendAttachment(
		context.Background(),
		SendAttachmentCommand{
			ConversationID:  conversationID,
			SenderID:        ownerID,
			ClientMessageID: "media-message-1",
			MessageType:     messageTypeImage,
			AttachmentID:    attachmentID,
			Content:         "",
		},
	)
	if err != nil {
		t.Fatalf("发送图片消息失败: %v", err)
	}
	var mediaRecord message
	if err := db.Where("id = ?", mediaMessage.MessageID).
		First(&mediaRecord).Error; err != nil {
		t.Fatalf("查询图片消息失败: %v", err)
	}
	if mediaRecord.MessageType != messageTypeImage ||
		mediaRecord.AttachmentID == nil ||
		*mediaRecord.AttachmentID != attachmentID {
		t.Fatalf("图片消息 = %+v", mediaRecord)
	}
	var senderReadSeq uint64
	if err := db.Table("conversation_members").
		Where("conversation_id = ? AND user_id = ?", conversationID, ownerID).
		Pluck("last_read_seq", &senderReadSeq).Error; err != nil {
		t.Fatalf("查询发送者已读游标失败: %v", err)
	}
	if senderReadSeq != mediaMessage.Seq {
		t.Fatalf("发送者已读游标 = %d，期望 %d", senderReadSeq, mediaMessage.Seq)
	}

	mediaHistory, err := service.ListMessages(
		context.Background(),
		ListMessagesCommand{
			ConversationID: conversationID,
			UserID:         ownerID,
			AfterSeq:       mediaMessage.Seq - 1,
			Limit:          1,
		},
	)
	if err != nil ||
		len(mediaHistory.Messages) != 1 ||
		mediaHistory.Messages[0].AttachmentName != "image.png" {
		t.Fatalf("图片消息历史 = %+v, err=%v", mediaHistory, err)
	}

	_, err = service.SendAttachment(
		context.Background(),
		SendAttachmentCommand{
			ConversationID:  conversationID,
			SenderID:        ownerID,
			ClientMessageID: "media-message-2",
			MessageType:     messageTypeFile,
			AttachmentID:    attachmentID,
			Content:         "",
		},
	)
	if !errors.Is(err, ErrAttachmentAlreadyUsed) {
		t.Fatalf("重复绑定附件错误 = %v，期望 %v", err, ErrAttachmentAlreadyUsed)
	}
}

func messageSequences(messages []MessageResult) []uint64 {
	sequences := make([]uint64, 0, len(messages))
	for _, message := range messages {
		sequences = append(sequences, message.Seq)
	}
	return sequences
}

func equalSequences(got []uint64, want []uint64) bool {
	if len(got) != len(want) {
		return false
	}
	for index := range want {
		if got[index] != want[index] {
			return false
		}
	}
	return true
}

func hasReadState(states []ReadStateResult, userID uint64, seq uint64) bool {
	for _, state := range states {
		if state.UserID == userID && state.LastReadSeq == seq {
			return true
		}
	}
	return false
}
