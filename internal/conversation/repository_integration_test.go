package conversation

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/PYang176/RelayIM/internal/platform/rabbitmq"
	"github.com/PYang176/RelayIM/internal/platform/snowflake"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var (
	conversationSnowflakeOnce sync.Once
	conversationSnowflakeErr  error
)

// initConversationSnowflake 初始化会话雪花算法运行状态
func initConversationSnowflake(t *testing.T) {
	t.Helper()
	conversationSnowflakeOnce.Do(func() {
		conversationSnowflakeErr = snowflake.Init(1)
	})
	if conversationSnowflakeErr != nil {
		t.Fatalf("初始化 Snowflake 失败: %v", conversationSnowflakeErr)
	}
}

// TestConversationRepositoryIntegration 覆盖会话、成员和权限规则的真实数据库链路
func TestConversationRepositoryIntegration(t *testing.T) {
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
	initConversationSnowflake(t)

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

	if err := db.Exec(
		"INSERT INTO user_blocks (blocker_id, blocked_id) VALUES (?, ?)",
		userA,
		userB,
	).Error; err != nil {
		t.Fatalf("创建私聊拉黑关系失败: %v", err)
	}
	if _, _, err := repository.CreateDirect(
		context.Background(),
		userA,
		userB,
	); !errors.Is(err, ErrUserBlocked) {
		t.Fatalf("拉黑后获取私聊错误 = %v，期望 ErrUserBlocked", err)
	}
	if err := db.Exec(
		"DELETE FROM user_blocks WHERE blocker_id = ? AND blocked_id = ?",
		userA,
		userB,
	).Error; err != nil {
		t.Fatalf("清理私聊拉黑关系失败: %v", err)
	}

	summaries, hasMore, err := repository.ListForUser(
		context.Background(),
		userA,
		ConversationPageQuery{Limit: defaultPageSize},
	)
	if err != nil {
		t.Fatalf("查询用户会话失败: %v", err)
	}
	if hasMore || len(summaries) != 1 || summaries[0].ID != first.ID {
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

	results, err := service.List(
		context.Background(),
		userA,
		ConversationPageQuery{Limit: defaultPageSize},
	)
	if err != nil {
		t.Fatalf("查询会话服务列表失败: %v", err)
	}
	if len(results.Conversations) != 1 ||
		results.Conversations[0].UnreadCount != 3 {
		t.Fatalf("会话列表 = %+v，期望未读数 3", results)
	}
	if results.Conversations[0].Title == "" {
		t.Fatalf("私聊标题 = %q，期望对方展示名称", results.Conversations[0].Title)
	}
	var friendshipID uint64
	lowID, highID := orderedUsers(userA, userB)
	if err := db.Table("friendships").
		Select("id").
		Where("user_low_id = ? AND user_high_id = ?", lowID, highID).
		Scan(&friendshipID).Error; err != nil || friendshipID == 0 {
		t.Fatalf("查询集成测试好友关系失败: %v", err)
	}
	if err := db.Exec(
		"INSERT INTO friend_remarks (friendship_id, user_id, remark) VALUES (?, ?, ?)",
		friendshipID,
		userA,
		"好友备注",
	).Error; err != nil {
		t.Fatalf("创建集成测试好友备注失败: %v", err)
	}
	remarked, err := service.List(
		context.Background(),
		userA,
		ConversationPageQuery{Limit: defaultPageSize},
	)
	if err != nil ||
		len(remarked.Conversations) != 1 ||
		remarked.Conversations[0].Title != "好友备注" {
		t.Fatalf("好友备注会话标题 = %+v, err=%v", remarked, err)
	}
	searched, err := service.List(
		context.Background(),
		userA,
		ConversationPageQuery{
			Limit:  defaultPageSize,
			Search: "好友备注",
		},
	)
	if err != nil ||
		len(searched.Conversations) != 1 ||
		searched.Conversations[0].ID != first.ID {
		t.Fatalf("好友备注搜索 = %+v, err=%v", searched, err)
	}

	group, err := service.CreateGroup(
		context.Background(),
		CreateGroupCommand{
			OwnerID:       userA,
			Name:          "集成测试群",
			MemberUserIDs: []uint64{userB},
			InviteRemark:  "邀请参与集成测试",
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
	invitationPage, err := service.ListReceivedInvitations(
		context.Background(),
		userB,
		InvitationPageQuery{Limit: defaultPageSize},
	)
	if err != nil ||
		len(invitationPage.Invitations) != 1 ||
		invitationPage.Invitations[0].ConversationID != group.ID ||
		invitationPage.Invitations[0].InviteRemark != "邀请参与集成测试" {
		t.Fatalf("群邀请列表 = %+v, err=%v", invitationPage, err)
	}
	accepted, err := service.AcceptInvitation(
		context.Background(),
		RespondInvitationCommand{
			UserID:         userB,
			InvitationID:   invitationPage.Invitations[0].InvitationID,
			ResponseRemark: "愿意加入集成测试群",
		},
	)
	if err != nil || accepted.MemberCount != 2 {
		t.Fatalf("接受群邀请 = %+v, err=%v", accepted, err)
	}
	if _, err := service.AcceptInvitation(
		context.Background(),
		RespondInvitationCommand{
			UserID:       userB,
			InvitationID: invitationPage.Invitations[0].InvitationID,
		},
	); err != nil {
		t.Fatalf("重复接受群邀请应保持幂等: %v", err)
	}
	groupInvitations, err := service.ListGroupInvitations(
		context.Background(),
		userA,
		group.ID,
		InvitationPageQuery{Limit: defaultPageSize},
	)
	if err != nil ||
		len(groupInvitations.Invitations) != 1 ||
		groupInvitations.Invitations[0].Status != invitationStatusAccepted ||
		groupInvitations.Invitations[0].ResponseRemark != "愿意加入集成测试群" {
		t.Fatalf("接受后的群邀请记录 = %+v, err=%v", groupInvitations, err)
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
			InviteRemark:  "非好友邀请测试",
		},
	)
	if !errors.Is(err, ErrNotFriend) {
		t.Fatalf("邀请非好友错误 = %v，期望 %v", err, ErrNotFriend)
	}

	invitedCount, err := service.InviteMembers(
		context.Background(),
		InviteMembersCommand{
			ActorID:        userA,
			ConversationID: group.ID,
			MemberUserIDs:  []uint64{userB},
			InviteRemark:   "重新邀请参与测试",
		},
	)
	if err != nil || invitedCount != 0 {
		t.Fatalf(
			"重复邀请成员 count=%d err=%v，期望有效成员不重复邀请",
			invitedCount,
			err,
		)
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

	invitedCount, err = service.InviteMembers(
		context.Background(),
		InviteMembersCommand{
			ActorID:        userA,
			ConversationID: group.ID,
			MemberUserIDs:  []uint64{userB},
			InviteRemark:   "再次邀请参与测试",
		},
	)
	if err != nil || invitedCount != 1 {
		t.Fatalf("重新邀请成员 count=%d err=%v", invitedCount, err)
	}
	invitationPage, err = service.ListReceivedInvitations(
		context.Background(),
		userB,
		InvitationPageQuery{Limit: defaultPageSize},
	)
	if err != nil || len(invitationPage.Invitations) != 1 {
		t.Fatalf("重新邀请列表 = %+v, err=%v", invitationPage, err)
	}
	if _, err := service.AcceptInvitation(
		context.Background(),
		RespondInvitationCommand{
			UserID:       userB,
			InvitationID: invitationPage.Invitations[0].InvitationID,
		},
	); err != nil {
		t.Fatalf("重新接受群邀请失败: %v", err)
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
			InviteRemark:   "解散后邀请测试",
		},
	); !errors.Is(err, ErrConversationNotFound) {
		t.Fatalf("解散后邀请成员错误 = %v，期望 ErrConversationNotFound", err)
	}
}

// TestConversationPaginationIntegration 覆盖会话列表复合游标分页的真实数据库行为
func TestConversationPaginationIntegration(t *testing.T) {
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
	initConversationSnowflake(t)

	userID, peerID := insertConversationTestUsers(t, db)
	t.Cleanup(func() {
		cleanupConversationTestData(t, db, userID, peerID)
	})

	const conversationCount = 5
	conversationIDs := make([]uint64, 0, conversationCount)
	baseTime := time.Now().UTC().Add(-time.Hour)
	for index := 0; index < conversationCount; index++ {
		conversationID := uint64(snowflake.Generate())
		conversationIDs = append(conversationIDs, conversationID)
		if err := db.Exec(
			`INSERT INTO conversations
				(id, conversation_type, owner_id, name, status, last_seq,
				 created_at, updated_at)
			 VALUES (?, 2, ?, ?, 1, 0, ?, ?)`,
			conversationID,
			userID,
			fmt.Sprintf("分页群%d", index),
			baseTime.Add(time.Duration(index)*time.Minute),
			baseTime.Add(time.Duration(index)*time.Minute),
		).Error; err != nil {
			t.Fatalf("创建分页会话失败: %v", err)
		}
		if err := db.Exec(
			`INSERT INTO conversation_members
				(conversation_id, user_id, role)
			 VALUES (?, ?, 3)`,
			conversationID,
			userID,
		).Error; err != nil {
			t.Fatalf("创建分页会话成员失败: %v", err)
		}
	}

	service := NewService(NewRepository(db))
	if err := service.SetPinned(
		context.Background(),
		userID,
		conversationIDs[0],
		true,
	); err != nil {
		t.Fatalf("置顶会话失败: %v", err)
	}
	if err := service.SetPinned(
		context.Background(),
		userID,
		conversationIDs[0],
		true,
	); err != nil {
		t.Fatalf("重复置顶会话应保持幂等: %v", err)
	}
	if err := service.SetPinned(
		context.Background(),
		peerID,
		conversationIDs[0],
		true,
	); !errors.Is(err, ErrConversationNotFound) {
		t.Fatalf("非成员置顶会话错误 = %v，期望 ErrConversationNotFound", err)
	}
	pinnedBoundary, err := service.List(
		context.Background(),
		userID,
		ConversationPageQuery{Limit: 1},
	)
	if err != nil ||
		len(pinnedBoundary.Conversations) != 1 ||
		pinnedBoundary.Conversations[0].ID != conversationIDs[0] ||
		!pinnedBoundary.Conversations[0].Pinned ||
		!pinnedBoundary.NextPinned {
		t.Fatalf("置顶会话边界页 = %+v, err=%v", pinnedBoundary, err)
	}
	unpinnedBoundary, err := service.List(
		context.Background(),
		userID,
		ConversationPageQuery{
			Limit:           1,
			BeforePinned:    pinnedBoundary.NextPinned,
			BeforeUpdatedAt: pinnedBoundary.NextUpdatedAt,
			BeforeID:        pinnedBoundary.NextID,
		},
	)
	if err != nil ||
		len(unpinnedBoundary.Conversations) != 1 ||
		unpinnedBoundary.Conversations[0].ID != conversationIDs[4] {
		t.Fatalf("置顶优先分页跨界 = %+v, err=%v", unpinnedBoundary, err)
	}
	first, err := service.List(
		context.Background(),
		userID,
		ConversationPageQuery{Limit: 2},
	)
	if err != nil ||
		len(first.Conversations) != 2 ||
		!first.HasMore ||
		first.NextID == 0 {
		t.Fatalf("会话第一页 = %+v, err=%v", first, err)
	}
	if first.Conversations[0].ID != conversationIDs[0] ||
		first.Conversations[1].ID != conversationIDs[4] {
		t.Fatalf("会话第一页顺序 = %+v", first.Conversations)
	}
	second, err := service.List(
		context.Background(),
		userID,
		ConversationPageQuery{
			Limit:           2,
			BeforePinned:    first.NextPinned,
			BeforeUpdatedAt: first.NextUpdatedAt,
			BeforeID:        first.NextID,
		},
	)
	if err != nil ||
		len(second.Conversations) != 2 ||
		!second.HasMore {
		t.Fatalf("会话第二页 = %+v, err=%v", second, err)
	}
	if second.Conversations[0].ID != conversationIDs[3] ||
		second.Conversations[1].ID != conversationIDs[2] {
		t.Fatalf("会话第二页顺序 = %+v", second.Conversations)
	}
	third, err := service.List(
		context.Background(),
		userID,
		ConversationPageQuery{
			Limit:           2,
			BeforePinned:    second.NextPinned,
			BeforeUpdatedAt: second.NextUpdatedAt,
			BeforeID:        second.NextID,
		},
	)
	if err != nil ||
		len(third.Conversations) != 1 ||
		third.HasMore ||
		third.Conversations[0].ID != conversationIDs[1] {
		t.Fatalf("会话第三页 = %+v, err=%v", third, err)
	}
	searchPage, err := service.List(
		context.Background(),
		userID,
		ConversationPageQuery{
			Limit:  2,
			Search: "分页群2",
		},
	)
	if err != nil ||
		len(searchPage.Conversations) != 1 ||
		searchPage.Conversations[0].ID != conversationIDs[2] {
		t.Fatalf("会话搜索 = %+v, err=%v", searchPage, err)
	}
	searchFirst, err := service.List(
		context.Background(),
		userID,
		ConversationPageQuery{
			Limit:  2,
			Search: "分页群",
		},
	)
	if err != nil ||
		len(searchFirst.Conversations) != 2 ||
		!searchFirst.HasMore {
		t.Fatalf("会话搜索第一页 = %+v, err=%v", searchFirst, err)
	}
	searchSecond, err := service.List(
		context.Background(),
		userID,
		ConversationPageQuery{
			Limit:           2,
			Search:          "分页群",
			BeforePinned:    searchFirst.NextPinned,
			BeforeUpdatedAt: searchFirst.NextUpdatedAt,
			BeforeID:        searchFirst.NextID,
		},
	)
	if err != nil ||
		len(searchSecond.Conversations) != 2 ||
		!searchSecond.HasMore {
		t.Fatalf("会话搜索第二页 = %+v, err=%v", searchSecond, err)
	}

	single, err := service.Get(
		context.Background(),
		userID,
		conversationIDs[2],
	)
	if err != nil || single.ID != conversationIDs[2] {
		t.Fatalf("单会话查询 = %+v, err=%v", single, err)
	}
	if err := service.SetPinned(
		context.Background(),
		userID,
		conversationIDs[0],
		false,
	); err != nil {
		t.Fatalf("取消置顶会话失败: %v", err)
	}
	unpinned, err := service.Get(
		context.Background(),
		userID,
		conversationIDs[0],
	)
	if err != nil || unpinned.Pinned {
		t.Fatalf("取消置顶后的会话 = %+v, err=%v", unpinned, err)
	}
}

// TestCreateDirectConcurrentIntegration 覆盖并发创建同一私聊会话时的唯一性约束
func TestCreateDirectConcurrentIntegration(t *testing.T) {
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
	initConversationSnowflake(t)

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

// TestGroupInvitationLifecycleIntegration 覆盖群邀请创建、审批、取消和过期的完整生命周期
func TestGroupInvitationLifecycleIntegration(t *testing.T) {
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
	initConversationSnowflake(t)

	ownerID, inviteeID := insertConversationTestUsers(t, db)
	t.Cleanup(func() {
		cleanupConversationTestData(t, db, ownerID, inviteeID)
	})
	service := NewService(NewRepository(db))
	group, err := service.CreateGroup(
		context.Background(),
		CreateGroupCommand{
			OwnerID:       ownerID,
			Name:          "邀请生命周期群",
			MemberUserIDs: []uint64{inviteeID},
			InviteRemark:  "邀请生命周期测试",
		},
	)
	if err != nil {
		t.Fatalf("创建邀请测试群失败: %v", err)
	}
	managedInvitations, err := service.ListGroupInvitations(
		context.Background(),
		ownerID,
		group.ID,
		InvitationPageQuery{Limit: defaultPageSize},
	)
	if err != nil || len(managedInvitations.Invitations) != 1 {
		t.Fatalf("群待审批邀请 = %+v, err=%v", managedInvitations, err)
	}
	if managedInvitations.Invitations[0].InviteRemark != "邀请生命周期测试" {
		t.Fatalf(
			"群待审批邀请备注 = %q",
			managedInvitations.Invitations[0].InviteRemark,
		)
	}
	if _, err := service.ListGroupInvitations(
		context.Background(),
		inviteeID,
		group.ID,
		InvitationPageQuery{Limit: defaultPageSize},
	); !errors.Is(err, ErrConversationNotFound) {
		t.Fatalf("非成员查询群邀请错误 = %v", err)
	}

	page, err := service.ListReceivedInvitations(
		context.Background(),
		inviteeID,
		InvitationPageQuery{Limit: defaultPageSize},
	)
	if err != nil || len(page.Invitations) != 1 {
		t.Fatalf("收到邀请 = %+v, err=%v", page, err)
	}
	invitationID := page.Invitations[0].InvitationID
	if _, err := service.AcceptInvitation(
		context.Background(),
		RespondInvitationCommand{
			UserID:       ownerID,
			InvitationID: invitationID,
		},
	); !errors.Is(err, ErrInvitationForbidden) {
		t.Fatalf("非接收者接受邀请错误 = %v", err)
	}

	if err := db.Table("group_invitations").
		Where("id = ?", invitationID).
		Updates(map[string]any{
			"created_at": time.Now().UTC().Add(-2 * time.Minute),
			"expires_at": time.Now().UTC().Add(-time.Minute),
		}).Error; err != nil {
		t.Fatalf("设置过期邀请失败: %v", err)
	}
	if _, err := service.AcceptInvitation(
		context.Background(),
		RespondInvitationCommand{
			UserID:       inviteeID,
			InvitationID: invitationID,
		},
	); !errors.Is(err, ErrInvitationExpired) {
		t.Fatalf("接受过期邀请错误 = %v", err)
	}

	invitedCount, err := service.InviteMembers(
		context.Background(),
		InviteMembersCommand{
			ActorID:        ownerID,
			ConversationID: group.ID,
			MemberUserIDs:  []uint64{inviteeID},
			InviteRemark:   "过期后重新邀请",
		},
	)
	if err != nil || invitedCount != 1 {
		t.Fatalf("重新邀请 count=%d err=%v", invitedCount, err)
	}
	if err := service.RejectInvitation(
		context.Background(),
		RespondInvitationCommand{
			UserID:         inviteeID,
			InvitationID:   invitationID,
			ResponseRemark: "当前不参加",
		},
	); err != nil {
		t.Fatalf("拒绝群邀请失败: %v", err)
	}
	page, err = service.ListReceivedInvitations(
		context.Background(),
		inviteeID,
		InvitationPageQuery{Limit: defaultPageSize},
	)
	if err != nil || len(page.Invitations) != 0 {
		t.Fatalf("拒绝后邀请列表 = %+v, err=%v", page, err)
	}
	groupInvitationHistory, err := service.ListGroupInvitations(
		context.Background(),
		ownerID,
		group.ID,
		InvitationPageQuery{Limit: defaultPageSize},
	)
	if err != nil ||
		len(groupInvitationHistory.Invitations) != 1 ||
		groupInvitationHistory.Invitations[0].Status != invitationStatusRejected ||
		groupInvitationHistory.Invitations[0].ResponseRemark != "当前不参加" {
		t.Fatalf("拒绝后的群邀请记录 = %+v, err=%v", groupInvitationHistory, err)
	}
	if _, err := service.InviteMembers(
		context.Background(),
		InviteMembersCommand{
			ActorID:        ownerID,
			ConversationID: group.ID,
			MemberUserIDs:  []uint64{inviteeID},
			InviteRemark:   "拒绝后再次邀请",
		},
	); err != nil {
		t.Fatalf("拒绝后重新邀请失败: %v", err)
	}
	if err := service.CancelInvitation(
		context.Background(),
		CancelInvitationCommand{
			ActorID:        ownerID,
			ConversationID: group.ID,
			InvitationID:   invitationID,
		},
	); err != nil {
		t.Fatalf("取消群邀请失败: %v", err)
	}
	if _, err := service.AcceptInvitation(
		context.Background(),
		RespondInvitationCommand{
			UserID:       inviteeID,
			InvitationID: invitationID,
		},
	); !errors.Is(err, ErrInvitationHandled) {
		t.Fatalf("取消后接受邀请错误 = %v", err)
	}
	if _, err := service.InviteMembers(
		context.Background(),
		InviteMembersCommand{
			ActorID:        ownerID,
			ConversationID: group.ID,
			MemberUserIDs:  []uint64{inviteeID},
			InviteRemark:   "取消后再次邀请",
		},
	); err != nil {
		t.Fatalf("取消后再次邀请失败: %v", err)
	}

	const workers = 2
	errs := make(chan error, workers)
	var waitGroup sync.WaitGroup
	for index := 0; index < workers; index++ {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			_, err := service.AcceptInvitation(
				context.Background(),
				RespondInvitationCommand{
					UserID:         inviteeID,
					InvitationID:   invitationID,
					ResponseRemark: "并发接受",
				},
			)
			errs <- err
		}()
	}
	waitGroup.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("并发接受邀请失败: %v", err)
		}
	}
	members, err := service.ListMembers(
		context.Background(),
		inviteeID,
		group.ID,
	)
	if err != nil || len(members) != 2 {
		t.Fatalf("接受后群成员 = %+v, err=%v", members, err)
	}
	managedInvitations, err = service.ListGroupInvitations(
		context.Background(),
		ownerID,
		group.ID,
		InvitationPageQuery{Limit: defaultPageSize},
	)
	if err != nil ||
		len(managedInvitations.Invitations) != 1 ||
		managedInvitations.Invitations[0].Status != invitationStatusAccepted ||
		managedInvitations.Invitations[0].ResponseRemark != "并发接受" {
		t.Fatalf("接受后群邀请记录 = %+v, err=%v", managedInvitations, err)
	}
	if _, err := service.ListGroupInvitations(
		context.Background(),
		inviteeID,
		group.ID,
		InvitationPageQuery{Limit: defaultPageSize},
	); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("普通成员查询群邀请错误 = %v", err)
	}
	var invitationEventCount int64
	if err := db.Table("outbox_events").
		Where(
			"aggregate_id = ? AND event_type IN ?",
			group.ID,
			[]string{
				rabbitmq.GroupInvitationCreatedRoutingKey,
				rabbitmq.GroupInvitationCancelledRoutingKey,
			},
		).
		Count(&invitationEventCount).Error; err != nil {
		t.Fatalf("查询群邀请 Outbox 事件失败: %v", err)
	}
	if invitationEventCount < 2 {
		t.Fatalf("群邀请 Outbox 事件数量 = %d，期望包含创建和取消", invitationEventCount)
	}
}

// insertConversationTestUsers 插入会话集成测试用户，并返回后续清理所需的测试上下文
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

// cleanupConversationTestData 清理会话集成测试创建的用户、会话和消息
func cleanupConversationTestData(
	t *testing.T,
	db *gorm.DB,
	userA uint64,
	userB uint64,
) {
	t.Helper()

	statements := []string{
		"DELETE FROM conversation_members WHERE user_id IN (?, ?)",
		"DELETE FROM group_invitations WHERE invitee_id IN (?, ?) " +
			"OR inviter_id IN (?, ?)",
		"DELETE FROM outbox_events WHERE aggregate_type = 'conversation' " +
			"AND aggregate_id IN (SELECT id FROM conversations " +
			"WHERE owner_id IN (?, ?) OR direct_low_user_id IN (?, ?) " +
			"OR direct_high_user_id IN (?, ?))",
		"DELETE FROM conversations WHERE owner_id IN (?, ?) " +
			"OR direct_low_user_id IN (?, ?) OR direct_high_user_id IN (?, ?)",
		"DELETE FROM user_blocks WHERE blocker_id IN (?, ?) OR blocked_id IN (?, ?)",
		"DELETE FROM friend_remarks WHERE user_id IN (?, ?)",
		"DELETE FROM friendships WHERE user_low_id IN (?, ?) OR user_high_id IN (?, ?)",
		"DELETE FROM user_profiles WHERE user_id IN (?, ?)",
		"DELETE FROM users WHERE id IN (?, ?)",
	}
	args := [][]any{
		{userA, userB},
		{userA, userB, userA, userB},
		{userA, userB, userA, userB, userA, userB},
		{userA, userB, userA, userB, userA, userB},
		{userA, userB, userA, userB},
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
