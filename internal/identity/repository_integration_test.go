package identity

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"ginChatProject/internal/platform/authn"
	"ginChatProject/internal/platform/snowflake"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type stubTokenIssuer struct {
	token          string
	err            error
	refreshCounter int
}

func (s *stubTokenIssuer) IssueToken(context.Context, uint64) (string, error) {
	return s.token, s.err
}

func (s *stubTokenIssuer) IssueRefreshToken(
	context.Context,
) (string, string, time.Time, error) {
	if s.err != nil {
		return "", "", time.Time{}, s.err
	}
	s.refreshCounter++
	raw := fmt.Sprintf("refresh-token-%d", s.refreshCounter)
	return raw, authn.HashRefreshToken(raw), time.Now().UTC().Add(24 * time.Hour), nil
}

func (s *stubTokenIssuer) Revoke(
	context.Context,
	string,
	string,
	time.Time,
) error {
	return nil
}

func TestUserRegisterAndLoginIntegration(t *testing.T) {
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

	username := fmt.Sprintf("it_%d", uint64(snowflake.Generate()))
	const password = "password123"
	tokenIssuer := &stubTokenIssuer{token: "test-token"}
	service := NewService(NewRepository(db), tokenIssuer)

	registered, err := service.Register(context.Background(), RegisterCommand{
		Username:        username,
		Password:        password,
		ConfirmPassword: password,
		Nickname:        "集成测试用户",
	})
	if err != nil {
		t.Fatalf("注册用户失败: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Exec("DELETE FROM refresh_tokens WHERE user_id = ?", registered.ID).Error; err != nil {
			t.Errorf("清理用户 Refresh Token 失败: %v", err)
		}
		if err := db.Exec("DELETE FROM devices WHERE user_id = ?", registered.ID).Error; err != nil {
			t.Errorf("清理用户设备失败: %v", err)
		}
		if err := db.Exec("DELETE FROM user_profiles WHERE user_id = ?", registered.ID).Error; err != nil {
			t.Errorf("清理用户资料失败: %v", err)
		}
		if err := db.Exec("DELETE FROM users WHERE id = ?", registered.ID).Error; err != nil {
			t.Errorf("清理用户失败: %v", err)
		}
	})

	// 重复用户名必须由数据库唯一索引兜底，即使并发检查同时通过。
	_, err = service.Register(context.Background(), RegisterCommand{
		Username:        username,
		Password:        password,
		ConfirmPassword: password,
	})
	if !errors.Is(err, ErrUsernameExists) {
		t.Fatalf("重复注册错误 = %v，期望 ErrUsernameExists", err)
	}

	login, err := service.Login(context.Background(), LoginCommand{
		Username: username,
		Password: password,
	})
	if err != nil {
		t.Fatalf("登录失败: %v", err)
	}
	if login.Token != "test-token" || login.User.ID != registered.ID {
		t.Fatalf("登录结果 = %+v，期望测试 Token 和注册用户 ID", login)
	}
	if login.RefreshToken == "" || login.DeviceID == 0 {
		t.Fatalf("登录结果缺少 Refresh Token 或设备 ID: %+v", login)
	}

	_, err = service.Login(context.Background(), LoginCommand{
		Username: username,
		Password: "wrong-password",
	})
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("错误密码登录错误 = %v，期望 ErrInvalidCredentials", err)
	}

	currentUser, err := service.Me(context.Background(), registered.ID)
	if err != nil {
		t.Fatalf("查询当前用户失败: %v", err)
	}
	if currentUser.Nickname != "集成测试用户" {
		t.Fatalf("当前用户昵称 = %q，期望集成测试用户", currentUser.Nickname)
	}
	updated, err := service.UpdateProfile(
		context.Background(),
		UpdateProfileCommand{
			UserID:    registered.ID,
			Nickname:  "更新后的昵称",
			AvatarURL: "https://example.com/avatar.png",
			Bio:       "正在完善 GinChat",
		},
	)
	if err != nil {
		t.Fatalf("更新用户资料失败: %v", err)
	}
	if updated.Nickname != "更新后的昵称" ||
		updated.AvatarURL != "https://example.com/avatar.png" ||
		updated.Bio != "正在完善 GinChat" {
		t.Fatalf("更新后用户资料 = %+v", updated)
	}
	reloaded, err := service.Me(context.Background(), registered.ID)
	if err != nil {
		t.Fatalf("重新读取更新后资料失败: %v", err)
	}
	if reloaded.Nickname != updated.Nickname ||
		reloaded.AvatarURL != updated.AvatarURL ||
		reloaded.Bio != updated.Bio {
		t.Fatalf("重新读取资料 = %+v，期望 %+v", reloaded, updated)
	}

	refreshed, err := service.Refresh(
		context.Background(),
		RefreshCommand{RefreshToken: login.RefreshToken},
	)
	if err != nil {
		t.Fatalf("轮换 Refresh Token 失败: %v", err)
	}
	if refreshed.RefreshToken == "" ||
		refreshed.RefreshToken == login.RefreshToken {
		t.Fatalf("Refresh Token 未轮换: %+v", refreshed)
	}
	if _, err := service.Refresh(
		context.Background(),
		RefreshCommand{RefreshToken: login.RefreshToken},
	); !errors.Is(err, ErrInvalidRefreshToken) {
		t.Fatalf("旧 Refresh Token 错误 = %v，期望 ErrInvalidRefreshToken", err)
	}

	if err := service.Logout(context.Background(), LogoutCommand{
		UserID:          registered.ID,
		AccessToken:     "test-token",
		JTI:             "test-jti",
		AccessExpiresAt: time.Now().UTC().Add(time.Hour),
		RefreshToken:    refreshed.RefreshToken,
	}); err != nil {
		t.Fatalf("退出登录失败: %v", err)
	}
	if _, err := service.Refresh(
		context.Background(),
		RefreshCommand{RefreshToken: refreshed.RefreshToken},
	); !errors.Is(err, ErrInvalidRefreshToken) {
		t.Fatalf("退出后 Refresh Token 错误 = %v，期望 ErrInvalidRefreshToken", err)
	}

	deviceResult, err := service.EnsureDevice(context.Background(), EnsureDeviceCommand{
		UserID:     registered.ID,
		DeviceName: "集成测试 Web",
		Platform:   1,
	})
	if err != nil {
		t.Fatalf("创建设备失败: %v", err)
	}
	if deviceResult.ID == 0 || deviceResult.DeviceName != "集成测试 Web" {
		t.Fatalf("设备结果 = %+v", deviceResult)
	}
	sameDevice, err := service.EnsureDevice(context.Background(), EnsureDeviceCommand{
		UserID:   registered.ID,
		DeviceID: deviceResult.ID,
	})
	if err != nil {
		var deviceCount int64
		countErr := db.Table("devices").
			Where("id = ? AND user_id = ?", deviceResult.ID, registered.ID).
			Count(&deviceCount).Error
		var activeCount int64
		activeErr := db.Table("devices").
			Where(
				"id = ? AND user_id = ? AND revoked_at IS NULL",
				deviceResult.ID,
				registered.ID,
			).
			Count(&activeCount).Error
		t.Fatalf(
			"复用设备失败: device_id=%d user_id=%d count=%d active=%d count_err=%v active_err=%v err=%v",
			deviceResult.ID,
			registered.ID,
			deviceCount,
			activeCount,
			countErr,
			activeErr,
			err,
		)
	}
	if sameDevice.ID != deviceResult.ID {
		t.Fatalf("复用设备 ID = %d，期望 %d", sameDevice.ID, deviceResult.ID)
	}
}
