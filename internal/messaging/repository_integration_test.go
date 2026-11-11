package messaging

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/PYang176/RelayIM/internal/platform/snowflake"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// TestRepositorySendTextIntegration 覆盖消息事务、序号、附件、Outbox 和搜索的完整链路
func TestRepositorySendTextIntegration(t *testing.T) {
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
	// 测试会真实并发调用 Snowflake，不能在多个测试二进制之间共享同一节点。
	if err := snowflake.Init(1); err != nil {
		t.Fatalf("初始化 Snowflake 失败: %v", err)
	}

	ownerID := uint64(snowflake.Generate())
	memberID := uint64(snowflake.Generate())
	outsiderID := uint64(snowflake.Generate())
	conversationID := uint64(snowflake.Generate())
	directConversationID := uint64(snowflake.Generate())
	directFriendshipID := uint64(snowflake.Generate())
	parallelConversationIDs := make([]uint64, 5)
	for index := range parallelConversationIDs {
		parallelConversationIDs[index] = uint64(snowflake.Generate())
	}
	memberDeviceID := uint64(snowflake.Generate())
	attachmentID := uint64(snowflake.Generate())
	oldOrphanID := uint64(snowflake.Generate())
	freshPendingID := uint64(snowflake.Generate())
	staleCleaningID := uint64(snowflake.Generate())
	userIDs := []uint64{ownerID, memberID, outsiderID}

	// 先注册清理函数，保证后续任何步骤失败时都不会把测试数据留在本地库。
	t.Cleanup(func() {
		cleanupStatements := []struct {
			query string
			args  []any
		}{
			{"DELETE FROM message_deliveries WHERE device_id = ?", []any{memberDeviceID}},
			{"DELETE FROM outbox_events WHERE aggregate_id = ?", []any{conversationID}},
			{"DELETE FROM outbox_events WHERE aggregate_id = ?", []any{directConversationID}},
			{"DELETE FROM messages WHERE conversation_id = ?", []any{conversationID}},
			{"DELETE FROM messages WHERE conversation_id = ?", []any{directConversationID}},
			{"DELETE FROM attachments WHERE id = ?", []any{attachmentID}},
			{
				"DELETE FROM attachments WHERE id IN (?, ?, ?)",
				[]any{oldOrphanID, freshPendingID, staleCleaningID},
			},
			{"DELETE FROM conversation_members WHERE conversation_id = ?", []any{conversationID}},
			{"DELETE FROM conversation_members WHERE conversation_id = ?", []any{directConversationID}},
			{"DELETE FROM conversations WHERE id = ?", []any{conversationID}},
			{"DELETE FROM conversations WHERE id = ?", []any{directConversationID}},
			{"DELETE FROM friendships WHERE id = ?", []any{directFriendshipID}},
			{"DELETE FROM devices WHERE id = ?", []any{memberDeviceID}},
			{
				"DELETE FROM user_blocks WHERE blocker_id IN (?, ?, ?) " +
					"OR blocked_id IN (?, ?, ?)",
				[]any{ownerID, memberID, outsiderID, ownerID, memberID, outsiderID},
			},
		}
		for _, parallelConversationID := range parallelConversationIDs {
			cleanupStatements = append(
				cleanupStatements,
				struct {
					query string
					args  []any
				}{
					query: "DELETE FROM outbox_events WHERE aggregate_id = ?",
					args:  []any{parallelConversationID},
				},
				struct {
					query string
					args  []any
				}{
					query: "DELETE FROM messages WHERE conversation_id = ?",
					args:  []any{parallelConversationID},
				},
				struct {
					query string
					args  []any
				}{
					query: "DELETE FROM conversation_members WHERE conversation_id = ?",
					args:  []any{parallelConversationID},
				},
				struct {
					query string
					args  []any
				}{
					query: "DELETE FROM conversations WHERE id = ?",
					args:  []any{parallelConversationID},
				},
			)
		}
		cleanupStatements = append(cleanupStatements, struct {
			query string
			args  []any
		}{
			query: "DELETE FROM users WHERE id IN (?, ?, ?)",
			args:  []any{ownerID, memberID, outsiderID},
		})
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
	cleanupNow := time.Now().UTC()
	for _, record := range []struct {
		id        uint64
		key       string
		status    int
		createdAt time.Time
		startedAt *time.Time
	}{
		{
			id:        oldOrphanID,
			key:       fmt.Sprintf("tests/%d/old.bin", oldOrphanID),
			status:    attachmentStatusPending,
			createdAt: cleanupNow.Add(-48 * time.Hour),
		},
		{
			id:        freshPendingID,
			key:       fmt.Sprintf("tests/%d/fresh.bin", freshPendingID),
			status:    attachmentStatusPending,
			createdAt: cleanupNow.Add(-time.Hour),
		},
		{
			id:        staleCleaningID,
			key:       fmt.Sprintf("tests/%d/stale.bin", staleCleaningID),
			status:    attachmentStatusCleaning,
			createdAt: cleanupNow.Add(-48 * time.Hour),
			startedAt: timePointer(cleanupNow.Add(-time.Hour)),
		},
	} {
		if err := db.Exec(
			`INSERT INTO attachments
				(id, owner_id, object_key, original_name, mime_type,
				 size_bytes, status, cleanup_started_at, created_at)
			 VALUES (?, ?, ?, ?, 'application/octet-stream', 4, ?, ?, ?)`,
			record.id,
			ownerID,
			record.key,
			"cleanup.bin",
			record.status,
			record.startedAt,
			record.createdAt,
		).Error; err != nil {
			t.Fatalf("创建孤儿附件清理测试数据失败: %v", err)
		}
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
	lowID, highID := min(ownerID, memberID), max(ownerID, memberID)
	if err := db.Exec(
		`INSERT INTO conversations
			(id, conversation_type, direct_low_user_id, direct_high_user_id, status, last_seq)
		 VALUES (?, 1, ?, ?, 1, 0)`,
		directConversationID,
		lowID,
		highID,
	).Error; err != nil {
		t.Fatalf("创建私聊测试会话失败: %v", err)
	}
	if err := db.Exec(
		`INSERT INTO conversation_members
			(conversation_id, user_id, role)
		 VALUES (?, ?, 1), (?, ?, 1)`,
		directConversationID,
		ownerID,
		directConversationID,
		memberID,
	).Error; err != nil {
		t.Fatalf("创建私聊测试成员失败: %v", err)
	}
	if err := db.Exec(
		`INSERT INTO friendships
			(id, user_low_id, user_high_id, status)
		 VALUES (?, ?, ?, 1)`,
		directFriendshipID,
		lowID,
		highID,
	).Error; err != nil {
		t.Fatalf("创建私聊好友关系失败: %v", err)
	}
	for _, parallelConversationID := range parallelConversationIDs {
		if err := db.Exec(
			`INSERT INTO conversations
				(id, conversation_type, owner_id, name, status, last_seq)
			 VALUES (?, 2, ?, ?, 1, 0)`,
			parallelConversationID,
			ownerID,
			fmt.Sprintf("并行发送测试群 %d", parallelConversationID),
		).Error; err != nil {
			t.Fatalf("创建并行发送测试会话失败: %v", err)
		}
		if err := db.Exec(
			`INSERT INTO conversation_members
				(conversation_id, user_id, role)
			 VALUES (?, ?, 3), (?, ?, 1)`,
			parallelConversationID,
			ownerID,
			parallelConversationID,
			memberID,
		).Error; err != nil {
			t.Fatalf("创建并行发送测试成员失败: %v", err)
		}
	}

	repository := NewRepository(db)
	service := NewService(repository)
	traceProvider := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)
	previousPropagator := otel.GetTextMapPropagator()
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() {
		otel.SetTextMapPropagator(previousPropagator)
		_ = traceProvider.Shutdown(context.Background())
	})
	sendCtx, sendSpan := traceProvider.Tracer("test").Start(
		context.Background(),
		"send-message",
	)
	// 场景 1：正常发送首条消息，验证 seq 从 1 开始且首次请求不标记重复。
	first, err := service.SendText(sendCtx, SendTextCommand{
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
	sendSpan.End()
	var persistedTraceParent string
	if err := db.Table("outbox_events").
		Where("aggregate_id = ? AND event_type = ?", conversationID, "message.created.v1").
		Order("id DESC").
		Pluck("trace_parent", &persistedTraceParent).Error; err != nil {
		t.Fatalf("查询 Outbox Trace 上下文失败: %v", err)
	}
	if persistedTraceParent == "" ||
		!strings.HasPrefix(persistedTraceParent, "00-") {
		t.Fatalf("Outbox trace_parent = %q", persistedTraceParent)
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

	if err := db.Exec(
		"INSERT INTO user_blocks (blocker_id, blocked_id) VALUES (?, ?)",
		ownerID,
		memberID,
	).Error; err != nil {
		t.Fatalf("创建消息拉黑关系失败: %v", err)
	}
	_, err = service.SendText(context.Background(), SendTextCommand{
		ConversationID:  directConversationID,
		SenderID:        memberID,
		ClientMessageID: "client-message-blocked",
		Content:         "拉黑后消息",
	})
	if !errors.Is(err, ErrUserBlocked) {
		t.Fatalf("拉黑后发送错误 = %v，期望 ErrUserBlocked", err)
	}
	if err := db.Exec(
		"DELETE FROM user_blocks WHERE blocker_id = ? AND blocked_id = ?",
		ownerID,
		memberID,
	).Error; err != nil {
		t.Fatalf("清理消息拉黑关系失败: %v", err)
	}
	if err := db.Exec(
		"UPDATE friendships SET low_user_deleted_at = CURRENT_TIMESTAMP(3) WHERE id = ?",
		directFriendshipID,
	).Error; err != nil {
		t.Fatalf("模拟单向删除好友失败: %v", err)
	}
	_, err = service.SendText(context.Background(), SendTextCommand{
		ConversationID:  directConversationID,
		SenderID:        memberID,
		ClientMessageID: "client-message-not-friends",
		Content:         "单向删除后消息",
	})
	if !errors.Is(err, ErrNotFriends) {
		t.Fatalf("单向删除后发送错误 = %v，期望 ErrNotFriends", err)
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

	// 场景 4b：同一发送者并发写入不同会话。唯一索引查询不能使用
	// FOR UPDATE，否则相邻 gap lock 会让互不相关的会话互相死锁。
	parallelErrs := make(chan error, len(parallelConversationIDs))
	startParallel := make(chan struct{})
	for index, parallelConversationID := range parallelConversationIDs {
		wg.Add(1)
		go func(index int, parallelConversationID uint64) {
			defer wg.Done()
			<-startParallel
			_, sendErr := service.SendText(
				context.Background(),
				SendTextCommand{
					ConversationID:  parallelConversationID,
					SenderID:        memberID,
					ClientMessageID: fmt.Sprintf("client-parallel-%d", index),
					Content:         fmt.Sprintf("跨会话并发消息 %d", index),
				},
			)
			if sendErr != nil {
				parallelErrs <- sendErr
			}
		}(index, parallelConversationID)
	}
	close(startParallel)
	wg.Wait()
	close(parallelErrs)
	for sendErr := range parallelErrs {
		t.Errorf("跨会话并发发送失败: %v", sendErr)
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

	singleCharacterPage, err := service.SearchMessages(
		context.Background(),
		SearchMessagesCommand{
			ConversationID: conversationID,
			UserID:         memberID,
			Query:          "并",
			Limit:          10,
		},
	)
	if err != nil {
		t.Fatalf("单字符搜索会话消息失败: %v", err)
	}
	if len(singleCharacterPage.Messages) != 5 {
		t.Fatalf("单字符搜索结果数量 = %d，期望 5", len(singleCharacterPage.Messages))
	}

	searchPage, err := service.SearchMessages(
		context.Background(),
		SearchMessagesCommand{
			ConversationID: conversationID,
			UserID:         memberID,
			Query:          "并发",
			Limit:          2,
		},
	)
	if err != nil {
		t.Fatalf("搜索会话消息失败: %v", err)
	}
	if len(searchPage.Messages) != 2 ||
		searchPage.Messages[0].Seq != 6 ||
		searchPage.Messages[1].Seq != 5 ||
		!searchPage.HasMore ||
		searchPage.NextBeforeSeq != 5 ||
		searchPage.Messages[0].SenderName == "" {
		t.Fatalf("消息搜索第一页 = %+v", searchPage)
	}
	searchNext, err := service.SearchMessages(
		context.Background(),
		SearchMessagesCommand{
			ConversationID: conversationID,
			UserID:         memberID,
			Query:          "并发",
			BeforeSeq:      searchPage.NextBeforeSeq,
			Limit:          2,
		},
	)
	if err != nil ||
		len(searchNext.Messages) != 2 ||
		searchNext.Messages[0].Seq != 4 ||
		searchNext.Messages[1].Seq != 3 {
		t.Fatalf("消息搜索第二页 = %+v, err=%v", searchNext, err)
	}
	if _, err := service.SearchMessages(
		context.Background(),
		SearchMessagesCommand{
			ConversationID: conversationID,
			UserID:         outsiderID,
			Query:          "并发",
			Limit:          2,
		},
	); !errors.Is(err, ErrNotMember) {
		t.Fatalf("非成员搜索消息错误 = %v，期望 ErrNotMember", err)
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

	if _, err := repository.findAccessibleAttachment(
		context.Background(),
		memberID,
		attachmentID,
	); err != nil {
		t.Fatalf("撤回前成员访问附件失败: %v", err)
	}
	if err := service.RecallMessage(
		context.Background(),
		RecallMessageCommand{
			ConversationID: conversationID,
			UserID:         memberID,
			MessageID:      mediaMessage.MessageID,
		},
	); !errors.Is(err, ErrRecallForbidden) {
		t.Fatalf("普通成员撤回他人消息错误 = %v，期望 ErrRecallForbidden", err)
	}
	if err := service.RecallMessage(
		context.Background(),
		RecallMessageCommand{
			ConversationID: conversationID,
			UserID:         ownerID,
			MessageID:      mediaMessage.MessageID,
		},
	); err != nil {
		t.Fatalf("群主撤回图片消息失败: %v", err)
	}
	// 重复撤回保持幂等，不重复写 Outbox。
	if err := service.RecallMessage(
		context.Background(),
		RecallMessageCommand{
			ConversationID: conversationID,
			UserID:         ownerID,
			MessageID:      mediaMessage.MessageID,
		},
	); err != nil {
		t.Fatalf("重复撤回图片消息失败: %v", err)
	}
	var recalledHistory MessagePage
	recalledHistory, err = service.ListMessages(
		context.Background(),
		ListMessagesCommand{
			ConversationID: conversationID,
			UserID:         memberID,
			AfterSeq:       mediaMessage.Seq - 1,
			Limit:          1,
		},
	)
	if err != nil ||
		len(recalledHistory.Messages) != 1 ||
		!recalledHistory.Messages[0].Recalled ||
		recalledHistory.Messages[0].Content != "" ||
		recalledHistory.Messages[0].AttachmentID != nil ||
		recalledHistory.Messages[0].AttachmentName != "" {
		t.Fatalf("撤回后历史消息 = %+v, err=%v", recalledHistory, err)
	}
	if _, err := repository.findAccessibleAttachment(
		context.Background(),
		memberID,
		attachmentID,
	); !errors.Is(err, ErrAttachmentNotFound) {
		t.Fatalf("撤回后成员访问附件错误 = %v，期望 ErrAttachmentNotFound", err)
	}
	var recallOutboxCount int64
	if err := db.Model(&outboxEvent{}).
		Where(
			"aggregate_id = ? AND event_type = ?",
			conversationID,
			"message.recalled.v1",
		).
		Count(&recallOutboxCount).Error; err != nil {
		t.Fatalf("查询撤回 Outbox 数量失败: %v", err)
	}
	if recallOutboxCount != 1 {
		t.Fatalf("撤回 Outbox 数量 = %d，期望 1", recallOutboxCount)
	}

	cleanupCandidates, err := repository.claimOrphanAttachments(
		context.Background(),
		cleanupNow,
		24*time.Hour,
		10*time.Minute,
		10,
	)
	if err != nil {
		t.Fatalf("领取孤儿附件失败: %v", err)
	}
	if len(cleanupCandidates) != 2 ||
		cleanupCandidates[0].ID != oldOrphanID ||
		cleanupCandidates[1].ID != staleCleaningID {
		t.Fatalf("孤儿附件候选 = %+v", cleanupCandidates)
	}
	var freshStatus int
	if err := db.Table("attachments").
		Where("id = ?", freshPendingID).
		Pluck("status", &freshStatus).Error; err != nil {
		t.Fatalf("查询新附件状态失败: %v", err)
	}
	if freshStatus != attachmentStatusPending {
		t.Fatalf("新附件状态 = %d，期望 %d", freshStatus, attachmentStatusPending)
	}

	var memberMessage message
	if err := db.Where(
		"conversation_id = ? AND sender_id = ?",
		conversationID,
		memberID,
	).Order("seq ASC").First(&memberMessage).Error; err != nil {
		t.Fatalf("查询成员消息失败: %v", err)
	}
	if err := repository.recallMessage(
		context.Background(),
		conversationID,
		memberID,
		memberMessage.ID,
		memberMessage.CreatedAt.Add(3*time.Minute),
	); !errors.Is(err, ErrRecallExpired) {
		t.Fatalf("普通成员超时撤回错误 = %v，期望 ErrRecallExpired", err)
	}
	if err := repository.recallMessage(
		context.Background(),
		conversationID,
		ownerID,
		memberMessage.ID,
		memberMessage.CreatedAt.Add(25*time.Hour),
	); !errors.Is(err, ErrRecallExpired) {
		t.Fatalf("群主超时撤回错误 = %v，期望 ErrRecallExpired", err)
	}
}

// messageSequences 提取测试消息列表中的会话序号
func messageSequences(messages []MessageResult) []uint64 {
	sequences := make([]uint64, 0, len(messages))
	for _, message := range messages {
		sequences = append(sequences, message.Seq)
	}
	return sequences
}

// equalSequences 判断两组消息序号是否完全一致
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

// timePointer 返回时间值指针，供测试构造可空时间字段
func timePointer(value time.Time) *time.Time {
	return &value
}

// hasReadState 判断ReadState是否存在
func hasReadState(states []ReadStateResult, userID uint64, seq uint64) bool {
	for _, state := range states {
		if state.UserID == userID && state.LastReadSeq == seq {
			return true
		}
	}
	return false
}
