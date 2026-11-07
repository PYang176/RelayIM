package identity

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

type stubTokenIssuer struct {
	token string
	err   error
}

func (s *stubTokenIssuer) IssueToken(context.Context, uint64) (string, error) {
	return s.token, s.err
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
		t.Fatalf("复用设备失败: %v", err)
	}
	if sameDevice.ID != deviceResult.ID {
		t.Fatalf("复用设备 ID = %d，期望 %d", sameDevice.ID, deviceResult.ID)
	}
}
