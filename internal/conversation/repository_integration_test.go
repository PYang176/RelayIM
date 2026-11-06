package conversation

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"

	"ginChatProject/internal/platform/snowflake"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestConversationRepositoryIntegration(t *testing.T) {
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

	userA, userB := insertConversationTestUsers(t, db)
	repository := NewRepository(db)
	service := NewService(repository)

	first, created, err := repository.CreateDirect(
		context.Background(),
		userA,
		userB,
	)
	if err != nil {
		t.Fatalf("首次创建私聊失败: %v", err)
	}
	if !created {
		t.Fatal("首次创建私聊 created = false，期望 true")
	}
	t.Cleanup(func() {
		cleanupConversationTestData(t, db, userA, userB)
	})

	second, created, err := repository.CreateDirect(
		context.Background(),
		userB,
		userA,
	)
	if err != nil {
		t.Fatalf("重复获取私聊失败: %v", err)
	}
	if created {
		t.Fatal("重复获取私聊 created = true，期望 false")
	}
	if second.ID != first.ID {
		t.Fatalf("重复获取会话 ID = %d，期望 %d", second.ID, first.ID)
	}

	summaries, err := repository.ListForUser(context.Background(), userA)
	if err != nil {
		t.Fatalf("查询用户会话失败: %v", err)
	}
	if len(summaries) != 1 || summaries[0].ID != first.ID {
		t.Fatalf("用户会话列表 = %+v，期望唯一会话 %d", summaries, first.ID)
	}

	if err := db.Table("conversations").
		Where("id = ?", first.ID).
		Update("last_seq", 5).Error; err != nil {
		t.Fatalf("更新会话消息序号失败: %v", err)
	}
	if err := db.Table("conversation_members").
		Where("conversation_id = ? AND user_id = ?", first.ID, userA).
		Update("last_read_seq", 2).Error; err != nil {
		t.Fatalf("更新已读序号失败: %v", err)
	}

	results, err := service.List(context.Background(), userA)
	if err != nil {
		t.Fatalf("查询会话服务列表失败: %v", err)
	}
	if len(results) != 1 || results[0].UnreadCount != 3 {
		t.Fatalf("会话列表 = %+v，期望未读数 3", results)
	}

	group, err := repository.CreateGroup(
		context.Background(),
		userA,
		"集成测试群",
		nil,
	)
	if err != nil {
		t.Fatalf("创建群会话失败: %v", err)
	}

	var ownerCount int64
	if err := db.Table("conversation_members").
		Where("conversation_id = ? AND user_id = ? AND role = ?",
			group.ID,
			userA,
			groupOwnerRole,
		).
		Count(&ownerCount).Error; err != nil {
		t.Fatalf("查询群主成员失败: %v", err)
	}
	if ownerCount != 1 {
		t.Fatalf("群主成员数量 = %d，期望 1", ownerCount)
	}
}

func TestCreateDirectConcurrentIntegration(t *testing.T) {
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

	userA, userB := insertConversationTestUsers(t, db)
	t.Cleanup(func() {
		cleanupConversationTestData(t, db, userA, userB)
	})

	repository := NewRepository(db)
	const workers = 2
	type createResult struct {
		id      uint64
		created bool
		err     error
	}

	results := make(chan createResult, workers)
	start := make(chan struct{})
	var waitGroup sync.WaitGroup
	for i := 0; i < workers; i++ {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			<-start

			record, created, err := repository.CreateDirect(
				context.Background(),
				userA,
				userB,
			)
			result := createResult{created: created, err: err}
			if record != nil {
				result.id = record.ID
			}
			results <- result
		}()
	}

	close(start)
	waitGroup.Wait()
	close(results)

	createdCount := 0
	var conversationID uint64
	for result := range results {
		if result.err != nil {
			t.Fatalf("并发创建私聊失败: %v", result.err)
		}
		if result.created {
			createdCount++
		}
		if conversationID == 0 {
			conversationID = result.id
		}
		if result.id != conversationID {
			t.Fatalf("并发创建返回不同会话 ID: %d 和 %d", conversationID, result.id)
		}
	}
	if createdCount != 1 {
		t.Fatalf("并发创建新会话次数 = %d，期望 1", createdCount)
	}
}

func insertConversationTestUsers(t *testing.T, db *gorm.DB) (uint64, uint64) {
	t.Helper()

	userA := uint64(snowflake.Generate())
	userB := uint64(snowflake.Generate())
	users := []struct {
		id       uint64
		username string
	}{
		{id: userA, username: fmt.Sprintf("conv_a_%d", userA)},
		{id: userB, username: fmt.Sprintf("conv_b_%d", userB)},
	}

	for _, user := range users {
		if err := db.Exec(
			"INSERT INTO users (id, username, password_hash, status) VALUES (?, ?, ?, 1)",
			user.id,
			user.username,
			"integration-test-hash",
		).Error; err != nil {
			t.Fatalf("创建集成测试用户失败: %v", err)
		}
	}
	return userA, userB
}

func cleanupConversationTestData(
	t *testing.T,
	db *gorm.DB,
	userA uint64,
	userB uint64,
) {
	t.Helper()

	statements := []string{
		"DELETE FROM conversation_members WHERE user_id IN (?, ?)",
		"DELETE FROM conversations WHERE owner_id IN (?, ?) " +
			"OR direct_low_user_id IN (?, ?) OR direct_high_user_id IN (?, ?)",
		"DELETE FROM users WHERE id IN (?, ?)",
	}
	args := [][]any{
		{userA, userB},
		{userA, userB, userA, userB, userA, userB},
		{userA, userB},
	}
	for i, statement := range statements {
		if err := db.Exec(statement, args[i]...).Error; err != nil {
			t.Errorf("清理会话集成测试数据失败: %v", err)
		}
	}
}
