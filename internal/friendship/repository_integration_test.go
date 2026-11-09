package friendship

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"

	"ginChatProject/internal/platform/snowflake"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestFriendshipRepositoryIntegration(t *testing.T) {
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

	userA := uint64(snowflake.Generate())
	userB := uint64(snowflake.Generate())
	userC := uint64(snowflake.Generate())
	t.Cleanup(func() {
		statements := []struct {
			query string
			args  []any
		}{
			{
				"DELETE FROM user_blocks WHERE blocker_id IN (?, ?, ?) " +
					"OR blocked_id IN (?, ?, ?)",
				[]any{userA, userB, userC, userA, userB, userC},
			},
			{
				"DELETE FROM friend_remarks WHERE user_id IN (?, ?, ?)",
				[]any{userA, userB, userC},
			},
			{
				"DELETE FROM friend_requests WHERE user_low_id IN (?, ?, ?) " +
					"OR user_high_id IN (?, ?, ?)",
				[]any{userA, userB, userC, userA, userB, userC},
			},
			{
				"DELETE FROM friendships WHERE user_low_id IN (?, ?, ?) " +
					"OR user_high_id IN (?, ?, ?)",
				[]any{userA, userB, userC, userA, userB, userC},
			},
			{"DELETE FROM user_profiles WHERE user_id IN (?, ?, ?)", []any{userA, userB, userC}},
			{"DELETE FROM users WHERE id IN (?, ?, ?)", []any{userA, userB, userC}},
		}
		for _, statement := range statements {
			if err := db.Exec(statement.query, statement.args...).Error; err != nil {
				t.Errorf("清理好友测试数据失败: %v", err)
			}
		}
	})

	users := []struct {
		id       uint64
		username string
		nickname string
	}{
		{id: userA, username: fmt.Sprintf("friend_a_%d", userA), nickname: "林然"},
		{id: userB, username: fmt.Sprintf("friend_b_%d", userB), nickname: "周屿"},
		{id: userC, username: fmt.Sprintf("friend_c_%d", userC), nickname: "测试用户"},
	}
	for _, user := range users {
		if err := db.Exec(
			"INSERT INTO users (id, username, password_hash, status) VALUES (?, ?, ?, 1)",
			user.id,
			user.username,
			"not-used",
		).Error; err != nil {
			t.Fatalf("创建好友测试用户失败: %v", err)
		}
		if err := db.Exec(
			"INSERT INTO user_profiles (user_id, nickname) VALUES (?, ?)",
			user.id,
			user.nickname,
		).Error; err != nil {
			t.Fatalf("创建好友测试资料失败: %v", err)
		}
	}

	service := NewService(NewRepository(db))
	searchResults, err := service.SearchUsers(
		context.Background(),
		userA,
		"周",
	)
	if err != nil {
		t.Fatalf("搜索用户失败: %v", err)
	}
	if len(searchResults) == 0 || searchResults[0].ID != userB {
		t.Fatalf("搜索结果 = %+v，期望包含用户 %d", searchResults, userB)
	}

	if err := service.SendRequest(
		context.Background(),
		userA,
		userB,
		"一起联调",
	); err != nil {
		t.Fatalf("发送好友申请失败: %v", err)
	}
	if err := service.SendRequest(
		context.Background(),
		userB,
		userA,
		"",
	); !errors.Is(err, ErrRequestPending) {
		t.Fatalf("反向重复申请错误 = %v，期望 ErrRequestPending", err)
	}

	received, err := service.ListRequests(context.Background(), userB, true)
	if err != nil || len(received) != 1 {
		t.Fatalf("收到好友申请 = %+v, err=%v", received, err)
	}
	if err := service.AcceptRequest(
		context.Background(),
		userB,
		received[0].ID,
	); err != nil {
		t.Fatalf("同意好友申请失败: %v", err)
	}

	friends, err := service.ListFriends(context.Background(), userA)
	if err != nil || len(friends) != 1 || friends[0].User.ID != userB {
		t.Fatalf("好友列表 = %+v, err=%v", friends, err)
	}
	if err := service.SetRemark(
		context.Background(),
		userA,
		userB,
		"产品-周屿",
	); err != nil {
		t.Fatalf("设置好友备注失败: %v", err)
	}
	friends, err = service.ListFriends(context.Background(), userA)
	if err != nil || friends[0].Remark != "产品-周屿" {
		t.Fatalf("好友备注列表 = %+v, err=%v", friends, err)
	}

	if err := service.BlockUser(context.Background(), userA, userB); err != nil {
		t.Fatalf("拉黑好友失败: %v", err)
	}
	if err := service.BlockUser(context.Background(), userA, userB); err != nil {
		t.Fatalf("重复拉黑应保持幂等: %v", err)
	}
	blockedUsers, err := service.ListBlockedUsers(context.Background(), userA)
	if err != nil || len(blockedUsers) != 1 || blockedUsers[0].User.ID != userB {
		t.Fatalf("拉黑列表 = %+v, err=%v", blockedUsers, err)
	}
	friends, err = service.ListFriends(context.Background(), userA)
	if err != nil || len(friends) != 1 || !friends[0].BlockedByMe {
		t.Fatalf("拉黑后的好友列表 = %+v, err=%v", friends, err)
	}

	if err := service.DeleteFriend(context.Background(), userA, userB); err != nil {
		t.Fatalf("删除好友失败: %v", err)
	}
	friends, err = service.ListFriends(context.Background(), userA)
	if err != nil || len(friends) != 0 {
		t.Fatalf("删除后好友列表 = %+v, err=%v", friends, err)
	}

	if err := service.SendRequest(
		context.Background(),
		userB,
		userA,
		"",
	); !errors.Is(err, ErrUserBlocked) {
		t.Fatalf("拉黑期间好友申请错误 = %v，期望 ErrUserBlocked", err)
	}
	if err := service.UnblockUser(context.Background(), userA, userB); err != nil {
		t.Fatalf("取消拉黑失败: %v", err)
	}

	if err := service.SendRequest(context.Background(), userA, userB, "重新添加"); err != nil {
		t.Fatalf("重新发送好友申请失败: %v", err)
	}
	received, err = service.ListRequests(context.Background(), userB, true)
	if err != nil || len(received) != 1 {
		t.Fatalf("重新申请列表 = %+v, err=%v", received, err)
	}
	if err := service.RejectRequest(
		context.Background(),
		userB,
		received[0].ID,
	); err != nil {
		t.Fatalf("拒绝好友申请失败: %v", err)
	}
	sent, err := service.ListRequests(context.Background(), userA, false)
	if err != nil || len(sent) != 1 || sent[0].Status != requestStatusRejected {
		t.Fatalf("发出申请列表 = %+v, err=%v", sent, err)
	}
}
