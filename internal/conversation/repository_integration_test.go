package conversation

import (
	"context"
	"errors"
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
	if results[0].Title == "" {
		t.Fatalf("私聊标题 = %q，期望对方展示名称", results[0].Title)
	}

	group, err := service.CreateGroup(
		context.Background(),
		CreateGroupCommand{
			OwnerID:       userA,
			Name:          "集成测试群",
			MemberUserIDs: []uint64{userB},
		},
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
	if group.Title != "集成测试群" {
		t.Fatalf("群会话标题 = %q，期望群名称", group.Title)
	}
	updatedGroup, err := service.UpdateGroupProfile(
		context.Background(),
		UpdateGroupProfileCommand{
			ActorID:        userA,
			ConversationID: group.ID,
			Name:           "更新后的群名",
			AvatarURL:      "https://example.com/group.png",
		},
	)
	if err != nil {
		t.Fatalf("更新群资料失败: %v", err)
	}
	if updatedGroup.Name != "更新后的群名" ||
		updatedGroup.AvatarURL != "https://example.com/group.png" {
		t.Fatalf("更新后的群资料 = %+v", updatedGroup)
	}
	if _, err := service.UpdateGroupProfile(
		context.Background(),
		UpdateGroupProfileCommand{
			ActorID:        userB,
			ConversationID: group.ID,
			Name:           "越权群名",
		},
	); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("普通成员修改群资料错误 = %v，期望 ErrPermissionDenied", err)
	}

	members, err := service.ListMembers(
		context.Background(),
		userB,
		group.ID,
	)
	if err != nil {
		t.Fatalf("查询群成员失败: %v", err)
	}
	if len(members) != 2 ||
		members[0].UserID != userA ||
		members[0].Role != groupOwnerRole {
		t.Fatalf("群成员列表 = %+v", members)
	}

	_, err = service.CreateGroup(
		context.Background(),
		CreateGroupCommand{
			OwnerID:       userA,
			Name:          "非好友群",
			MemberUserIDs: []uint64{uint64(snowflake.Generate())},
		},
	)
	if !errors.Is(err, ErrNotFriend) {
		t.Fatalf("邀请非好友错误 = %v，期望 %v", err, ErrNotFriend)
	}

	memberCount, err := service.InviteMembers(
		context.Background(),
		InviteMembersCommand{
			ActorID:        userA,
			ConversationID: group.ID,
			MemberUserIDs:  []uint64{userB},
		},
	)
	if err != nil || memberCount != 2 {
		t.Fatalf("重复邀请成员 count=%d err=%v，期望幂等返回 2", memberCount, err)
	}

	if err := service.RemoveMember(
		context.Background(),
		RemoveMemberCommand{
			ActorID:        userB,
			ConversationID: group.ID,
			TargetUserID:   userB,
		},
	); err != nil {
		t.Fatalf("普通成员退群失败: %v", err)
	}
	if _, err := service.ListMembers(
		context.Background(),
		userB,
		group.ID,
	); !errors.Is(err, ErrConversationNotFound) {
		t.Fatalf("退群后读取成员错误 = %v，期望 ErrConversationNotFound", err)
	}
	if err := service.RemoveMember(
		context.Background(),
		RemoveMemberCommand{
			ActorID:        userA,
			ConversationID: group.ID,
			TargetUserID:   userA,
		},
	); !errors.Is(err, ErrOwnerCannotLeave) {
		t.Fatalf("群主退群错误 = %v，期望 ErrOwnerCannotLeave", err)
	}

	memberCount, err = service.InviteMembers(
		context.Background(),
		InviteMembersCommand{
			ActorID:        userA,
			ConversationID: group.ID,
			MemberUserIDs:  []uint64{userB},
		},
	)
	if err != nil || memberCount != 2 {
		t.Fatalf("重新邀请成员 count=%d err=%v", memberCount, err)
	}
	if err := service.RemoveMember(
		context.Background(),
		RemoveMemberCommand{
			ActorID:        userB,
			ConversationID: group.ID,
			TargetUserID:   userA,
		},
	); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("普通成员移除群主错误 = %v，期望 ErrPermissionDenied", err)
	}

	if err := service.SetMemberRole(
		context.Background(),
		SetMemberRoleCommand{
			ActorID:        userA,
			ConversationID: group.ID,
			TargetUserID:   userB,
			Role:           groupAdminRole,
		},
	); err != nil {
		t.Fatalf("设置群管理员失败: %v", err)
	}
	members, err = service.ListMembers(context.Background(), userA, group.ID)
	if err != nil {
		t.Fatalf("设置管理员后查询成员失败: %v", err)
	}
	if members[1].UserID != userB || members[1].Role != groupAdminRole {
		t.Fatalf("管理员角色未生效: %+v", members)
	}
	if err := service.SetMemberRole(
		context.Background(),
		SetMemberRoleCommand{
			ActorID:        userB,
			ConversationID: group.ID,
			TargetUserID:   userA,
			Role:           groupMemberRole,
		},
	); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("管理员修改群主角色错误 = %v，期望 ErrPermissionDenied", err)
	}

	if err := service.TransferOwner(
		context.Background(),
		TransferOwnerCommand{
			ActorID:        userA,
			ConversationID: group.ID,
			TargetUserID:   userB,
		},
	); err != nil {
		t.Fatalf("转让群主失败: %v", err)
	}
	members, err = service.ListMembers(context.Background(), userB, group.ID)
	if err != nil {
		t.Fatalf("转让群主后查询成员失败: %v", err)
	}
	if members[0].UserID != userB || members[0].Role != groupOwnerRole {
		t.Fatalf("新群主未排在首位: %+v", members)
	}
	if members[1].UserID != userA || members[1].Role != groupMemberRole {
		t.Fatalf("原群主未降级为普通成员: %+v", members)
	}
	var ownerID uint64
	if err := db.Table("conversations").
		Select("owner_id").
		Where("id = ?", group.ID).
		Scan(&ownerID).Error; err != nil {
		t.Fatalf("查询群主字段失败: %v", err)
	}
	if ownerID != userB {
		t.Fatalf("会话群主 = %d，期望 %d", ownerID, userB)
	}
	if err := service.DissolveGroup(
		context.Background(),
		DissolveGroupCommand{
			ActorID:        userA,
			ConversationID: group.ID,
		},
	); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("原群主解散群错误 = %v，期望 ErrPermissionDenied", err)
	}
	if err := service.DissolveGroup(
		context.Background(),
		DissolveGroupCommand{
			ActorID:        userB,
			ConversationID: group.ID,
		},
	); err != nil {
		t.Fatalf("新群主解散群失败: %v", err)
	}
	if _, err := service.ListMembers(
		context.Background(),
		userA,
		group.ID,
	); !errors.Is(err, ErrConversationNotFound) {
		t.Fatalf("解散后查询成员错误 = %v，期望 ErrConversationNotFound", err)
	}
	if _, err := service.InviteMembers(
		context.Background(),
		InviteMembersCommand{
			ActorID:        userB,
			ConversationID: group.ID,
			MemberUserIDs:  []uint64{userA},
		},
	); !errors.Is(err, ErrConversationNotFound) {
		t.Fatalf("解散后邀请成员错误 = %v，期望 ErrConversationNotFound", err)
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
		if err := db.Exec(
			"INSERT INTO user_profiles (user_id, nickname) VALUES (?, ?)",
			user.id,
			user.username,
		).Error; err != nil {
			t.Fatalf("创建集成测试用户资料失败: %v", err)
		}
	}
	friendshipID := uint64(snowflake.Generate())
	friendLowID, friendHighID := orderedUsers(userA, userB)
	if err := db.Exec(
		`INSERT INTO friendships
			(id, user_low_id, user_high_id, status)
		 VALUES (?, ?, ?, 1)`,
		friendshipID,
		friendLowID,
		friendHighID,
	).Error; err != nil {
		t.Fatalf("创建集成测试好友关系失败: %v", err)
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
		"DELETE FROM friend_remarks WHERE user_id IN (?, ?)",
		"DELETE FROM friendships WHERE user_low_id IN (?, ?) OR user_high_id IN (?, ?)",
		"DELETE FROM user_profiles WHERE user_id IN (?, ?)",
		"DELETE FROM users WHERE id IN (?, ?)",
	}
	args := [][]any{
		{userA, userB},
		{userA, userB, userA, userB, userA, userB},
		{userA, userB},
		{userA, userB, userA, userB},
		{userA, userB},
		{userA, userB},
	}
	for i, statement := range statements {
		if err := db.Exec(statement, args[i]...).Error; err != nil {
			t.Errorf("清理会话集成测试数据失败: %v", err)
		}
	}
}
