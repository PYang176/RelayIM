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

// TestFriendshipRepositoryIntegration 覆盖好友申请、关系、备注和拉黑的数据一致性
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
	page := PageQuery{Limit: defaultPageSize}
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
		"反向重复申请",
	); !errors.Is(err, ErrRequestPending) {
		t.Fatalf("反向重复申请错误 = %v，期望 ErrRequestPending", err)
	}

	received, err := service.ListRequests(context.Background(), userB, true, page)
	if err != nil || len(received.Requests) != 1 {
		t.Fatalf("收到好友申请 = %+v, err=%v", received, err)
	}
	if err := service.AcceptRequest(
		context.Background(),
		userB,
		received.Requests[0].ID,
	); err != nil {
		t.Fatalf("同意好友申请失败: %v", err)
	}

	friends, err := service.ListFriends(context.Background(), userA, page)
	if err != nil ||
		len(friends.Friends) != 1 ||
		friends.Friends[0].User.ID != userB {
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
	friends, err = service.ListFriends(context.Background(), userA, page)
	if err != nil || friends.Friends[0].Remark != "产品-周屿" {
		t.Fatalf("好友备注列表 = %+v, err=%v", friends, err)
	}

	if err := service.BlockUser(context.Background(), userA, userB); err != nil {
		t.Fatalf("拉黑好友失败: %v", err)
	}
	if err := service.BlockUser(context.Background(), userA, userB); err != nil {
		t.Fatalf("重复拉黑应保持幂等: %v", err)
	}
	blockedUsers, err := service.ListBlockedUsers(
		context.Background(),
		userA,
		page,
	)
	if err != nil ||
		len(blockedUsers.BlockedUsers) != 1 ||
		blockedUsers.BlockedUsers[0].User.ID != userB {
		t.Fatalf("拉黑列表 = %+v, err=%v", blockedUsers, err)
	}
	friends, err = service.ListFriends(context.Background(), userA, page)
	if err != nil ||
		len(friends.Friends) != 1 ||
		!friends.Friends[0].BlockedByMe {
		t.Fatalf("拉黑后的好友列表 = %+v, err=%v", friends, err)
	}

	if err := service.DeleteFriend(context.Background(), userA, userB); err != nil {
		t.Fatalf("删除好友失败: %v", err)
	}
	friends, err = service.ListFriends(context.Background(), userA, page)
	if err != nil || len(friends.Friends) != 0 {
		t.Fatalf("删除后好友列表 = %+v, err=%v", friends, err)
	}

	if err := service.SendRequest(
		context.Background(),
		userB,
		userA,
		"拉黑期间申请",
	); !errors.Is(err, ErrUserBlocked) {
		t.Fatalf("拉黑期间好友申请错误 = %v，期望 ErrUserBlocked", err)
	}
	if err := service.UnblockUser(context.Background(), userA, userB); err != nil {
		t.Fatalf("取消拉黑失败: %v", err)
	}

	if err := service.SendRequest(context.Background(), userA, userB, "重新添加"); err != nil {
		t.Fatalf("重新发送好友申请失败: %v", err)
	}
	received, err = service.ListRequests(context.Background(), userB, true, page)
	if err != nil || len(received.Requests) != 1 {
		t.Fatalf("重新申请列表 = %+v, err=%v", received, err)
	}
	if err := service.RejectRequest(
		context.Background(),
		userB,
		received.Requests[0].ID,
	); err != nil {
		t.Fatalf("拒绝好友申请失败: %v", err)
	}
	sent, err := service.ListRequests(context.Background(), userA, false, page)
	if err != nil ||
		len(sent.Requests) != 1 ||
		sent.Requests[0].Status != requestStatusRejected {
		t.Fatalf("发出申请列表 = %+v, err=%v", sent, err)
	}
}

// TestFriendshipPaginationIntegration 覆盖好友、申请和拉黑列表的游标分页
func TestFriendshipPaginationIntegration(t *testing.T) {
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

	const peerCount = 5
	userIDs := []uint64{uint64(snowflake.Generate())}
	for index := 0; index < peerCount; index++ {
		userIDs = append(userIDs, uint64(snowflake.Generate()))
	}
	t.Cleanup(func() {
		statements := []string{
			"DELETE FROM friend_remarks WHERE user_id IN (?, ?, ?, ?, ?, ?)",
			"DELETE FROM friend_requests WHERE user_low_id IN (?, ?, ?, ?, ?, ?) " +
				"OR user_high_id IN (?, ?, ?, ?, ?, ?)",
			"DELETE FROM friendships WHERE user_low_id IN (?, ?, ?, ?, ?, ?) " +
				"OR user_high_id IN (?, ?, ?, ?, ?, ?)",
			"DELETE FROM user_blocks WHERE blocker_id IN (?, ?, ?, ?, ?, ?) " +
				"OR blocked_id IN (?, ?, ?, ?, ?, ?)",
			"DELETE FROM user_profiles WHERE user_id IN (?, ?, ?, ?, ?, ?)",
			"DELETE FROM users WHERE id IN (?, ?, ?, ?, ?, ?)",
		}
		args := []any{userIDs[0], userIDs[1], userIDs[2], userIDs[3], userIDs[4], userIDs[5]}
		args12 := append(append([]any{}, args...), args...)
		argsList := [][]any{
			args,
			args12,
			args12,
			args12,
			args,
			args,
		}
		for index, statement := range statements {
			if err := db.Exec(statement, argsList[index]...).Error; err != nil {
				t.Errorf("清理分页测试数据失败: %v", err)
			}
		}
	})

	for index, userID := range userIDs {
		if err := db.Exec(
			"INSERT INTO users (id, username, password_hash, status) VALUES (?, ?, ?, 1)",
			userID,
			fmt.Sprintf("page_%d_%d", index, userID),
			"not-used",
		).Error; err != nil {
			t.Fatalf("创建分页测试用户失败: %v", err)
		}
		if err := db.Exec(
			"INSERT INTO user_profiles (user_id, nickname) VALUES (?, ?)",
			userID,
			fmt.Sprintf("分页用户%d", index),
		).Error; err != nil {
			t.Fatalf("创建分页测试资料失败: %v", err)
		}
	}

	centralID := userIDs[0]
	for _, peerID := range userIDs[1:] {
		lowID, highID := orderedUsers(centralID, peerID)
		if err := db.Exec(
			`INSERT INTO friendships
				(id, user_low_id, user_high_id, status)
			 VALUES (?, ?, ?, 1)`,
			uint64(snowflake.Generate()),
			lowID,
			highID,
		).Error; err != nil {
			t.Fatalf("创建分页好友关系失败: %v", err)
		}
	}

	service := NewService(NewRepository(db))
	query := PageQuery{Limit: 2}
	firstFriends, err := service.ListFriends(context.Background(), centralID, query)
	if err != nil ||
		len(firstFriends.Friends) != 2 ||
		!firstFriends.HasMore ||
		firstFriends.NextCursor == 0 {
		t.Fatalf("好友第一页 = %+v, err=%v", firstFriends, err)
	}
	secondFriends, err := service.ListFriends(
		context.Background(),
		centralID,
		PageQuery{Limit: 2, BeforeID: firstFriends.NextCursor},
	)
	if err != nil ||
		len(secondFriends.Friends) != 2 ||
		!secondFriends.HasMore {
		t.Fatalf("好友第二页 = %+v, err=%v", secondFriends, err)
	}
	thirdFriends, err := service.ListFriends(
		context.Background(),
		centralID,
		PageQuery{Limit: 2, BeforeID: secondFriends.NextCursor},
	)
	if err != nil ||
		len(thirdFriends.Friends) != 1 ||
		thirdFriends.HasMore ||
		thirdFriends.NextCursor != 0 {
		t.Fatalf("好友第三页 = %+v, err=%v", thirdFriends, err)
	}

	for _, peerID := range userIDs[1:] {
		if err := db.Exec(
			"INSERT INTO user_blocks (blocker_id, blocked_id) VALUES (?, ?)",
			centralID,
			peerID,
		).Error; err != nil {
			t.Fatalf("创建分页拉黑关系失败: %v", err)
		}
	}
	firstBlocked, err := service.ListBlockedUsers(
		context.Background(),
		centralID,
		query,
	)
	if err != nil ||
		len(firstBlocked.BlockedUsers) != 2 ||
		!firstBlocked.HasMore ||
		firstBlocked.NextCursor == 0 {
		t.Fatalf("拉黑第一页 = %+v, err=%v", firstBlocked, err)
	}
	secondBlocked, err := service.ListBlockedUsers(
		context.Background(),
		centralID,
		PageQuery{Limit: 2, BeforeID: firstBlocked.NextCursor},
	)
	if err != nil ||
		len(secondBlocked.BlockedUsers) != 2 ||
		!secondBlocked.HasMore {
		t.Fatalf("拉黑第二页 = %+v, err=%v", secondBlocked, err)
	}
	thirdBlocked, err := service.ListBlockedUsers(
		context.Background(),
		centralID,
		PageQuery{Limit: 2, BeforeID: secondBlocked.NextCursor},
	)
	if err != nil ||
		len(thirdBlocked.BlockedUsers) != 1 ||
		thirdBlocked.HasMore ||
		thirdBlocked.NextCursor != 0 {
		t.Fatalf("拉黑第三页 = %+v, err=%v", thirdBlocked, err)
	}

	for _, peerID := range userIDs[1:] {
		lowID, highID := orderedUsers(centralID, peerID)
		if err := db.Exec(
			`INSERT INTO friend_requests
				(id, user_low_id, user_high_id, requester_id, status, remark)
			 VALUES (?, ?, ?, ?, 1, ?)`,
			uint64(snowflake.Generate()),
			lowID,
			highID,
			peerID,
			"分页申请",
		).Error; err != nil {
			t.Fatalf("创建分页好友申请失败: %v", err)
		}
	}

	firstReceived, err := service.ListRequests(
		context.Background(),
		centralID,
		true,
		query,
	)
	if err != nil ||
		len(firstReceived.Requests) != 2 ||
		!firstReceived.HasMore ||
		firstReceived.NextCursor == 0 {
		t.Fatalf("收到申请第一页 = %+v, err=%v", firstReceived, err)
	}
	secondReceived, err := service.ListRequests(
		context.Background(),
		centralID,
		true,
		PageQuery{Limit: 2, BeforeID: firstReceived.NextCursor},
	)
	if err != nil ||
		len(secondReceived.Requests) != 2 ||
		!secondReceived.HasMore {
		t.Fatalf("收到申请第二页 = %+v, err=%v", secondReceived, err)
	}
	thirdReceived, err := service.ListRequests(
		context.Background(),
		centralID,
		true,
		PageQuery{Limit: 2, BeforeID: secondReceived.NextCursor},
	)
	if err != nil ||
		len(thirdReceived.Requests) != 1 ||
		thirdReceived.HasMore ||
		thirdReceived.NextCursor != 0 {
		t.Fatalf("收到申请第三页 = %+v, err=%v", thirdReceived, err)
	}

	if err := db.Exec(
		"DELETE FROM friend_requests WHERE user_low_id = ? OR user_high_id = ?",
		centralID,
		centralID,
	).Error; err != nil {
		t.Fatalf("清理收到申请分页数据失败: %v", err)
	}
	for _, peerID := range userIDs[1:] {
		lowID, highID := orderedUsers(centralID, peerID)
		if err := db.Exec(
			`INSERT INTO friend_requests
				(id, user_low_id, user_high_id, requester_id, status, remark)
			 VALUES (?, ?, ?, ?, 1, ?)`,
			uint64(snowflake.Generate()),
			lowID,
			highID,
			centralID,
			"发出分页申请",
		).Error; err != nil {
			t.Fatalf("创建发出分页申请失败: %v", err)
		}
	}
	firstSent, err := service.ListRequests(
		context.Background(),
		centralID,
		false,
		query,
	)
	if err != nil ||
		len(firstSent.Requests) != 2 ||
		!firstSent.HasMore {
		t.Fatalf("发出申请第一页 = %+v, err=%v", firstSent, err)
	}
	secondSent, err := service.ListRequests(
		context.Background(),
		centralID,
		false,
		PageQuery{Limit: 2, BeforeID: firstSent.NextCursor},
	)
	if err != nil ||
		len(secondSent.Requests) != 2 ||
		!secondSent.HasMore {
		t.Fatalf("发出申请第二页 = %+v, err=%v", secondSent, err)
	}
	thirdSent, err := service.ListRequests(
		context.Background(),
		centralID,
		false,
		PageQuery{Limit: 2, BeforeID: secondSent.NextCursor},
	)
	if err != nil ||
		len(thirdSent.Requests) != 1 ||
		thirdSent.HasMore ||
		thirdSent.NextCursor != 0 {
		t.Fatalf("发出申请第三页 = %+v, err=%v", thirdSent, err)
	}
}
