package identity

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"ginChatProject/internal/platform/security"
	"ginChatProject/internal/platform/snowflake"
)

const (
	minPasswordBytes = 8
	maxPasswordBytes = 72
	maxNicknameRunes = 64
	minUsernameRunes = 2
	maxUsernameRunes = 32
)

var usernamePattern = regexp.MustCompile(`^[\p{L}\p{N}_-]+$`)

// Service 提供用户注册、登录和当前用户查询。
type Service struct {
	repo  *Repository
	token TokenIssuer
}

// NewService 创建用户认证服务。
func NewService(repo *Repository, token TokenIssuer) *Service {
	return &Service{repo: repo, token: token}
}

// RegisterCommand 是注册用户所需的业务参数。
type RegisterCommand struct {
	// Username 是唯一登录名。
	Username string

	// Password 是明文密码，只在内存中用于哈希。
	Password string

	// ConfirmPassword 用于确认两次输入一致。
	ConfirmPassword string

	// Nickname 是公开昵称，为空时使用用户名。
	Nickname string
}

// UserResult 是可安全返回给接口层的用户信息。
type UserResult struct {
	// ID 是 Snowflake 用户 ID。
	ID uint64

	// Username 是登录名。
	Username string

	// Nickname 是公开昵称。
	Nickname string
}

// LoginCommand 是用户登录参数。
type LoginCommand struct {
	// Username 是登录名。
	Username string

	// Password 是待校验的明文密码。
	Password string
}

// LoginResult 是登录成功结果。
type LoginResult struct {
	// Token 是访问受保护接口的 JWT。
	Token string

	// User 是脱敏后的用户信息。
	User UserResult
}

// Register 创建用户身份和默认资料。
func (s *Service) Register(
	ctx context.Context,
	command RegisterCommand,
) (UserResult, error) {
	command.Username = strings.TrimSpace(command.Username)
	command.Nickname = strings.TrimSpace(command.Nickname)

	// 输入校验在哈希和数据库事务前完成，避免无效请求消耗 bcrypt 和连接资源。
	if !validUsername(command.Username) {
		return UserResult{}, ErrInvalidInput
	}
	if command.Password != command.ConfirmPassword ||
		len(command.Password) < minPasswordBytes ||
		len(command.Password) > maxPasswordBytes {
		return UserResult{}, ErrInvalidInput
	}
	if command.Nickname == "" {
		command.Nickname = command.Username
	}
	if utf8.RuneCountInString(command.Nickname) > maxNicknameRunes {
		return UserResult{}, ErrInvalidInput
	}

	passwordHash, err := security.HashPassword(command.Password)
	if err != nil {
		return UserResult{}, fmt.Errorf("生成密码哈希失败: %w", err)
	}

	now := time.Now().UTC()
	userRecord := &user{
		ID:           uint64(snowflake.Generate()),
		Username:     command.Username,
		PasswordHash: passwordHash,
		Status:       userStatusActive,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	profile := &userProfile{
		UserID:    userRecord.ID,
		Nickname:  command.Nickname,
		CreatedAt: now,
		UpdatedAt: now,
	}

	// Repository 在同一事务中保存 users 和 user_profiles。
	if err := s.repo.CreateUser(ctx, userRecord, profile); err != nil {
		return UserResult{}, err
	}

	return UserResult{
		ID:       userRecord.ID,
		Username: userRecord.Username,
		Nickname: profile.Nickname,
	}, nil
}

// Login 校验密码并签发可撤销的 JWT。
func (s *Service) Login(
	ctx context.Context,
	command LoginCommand,
) (LoginResult, error) {
	command.Username = strings.TrimSpace(command.Username)
	if command.Username == "" || command.Password == "" {
		return LoginResult{}, ErrInvalidCredentials
	}

	userRecord, err := s.repo.FindByUsername(ctx, command.Username)
	if err != nil {
		return LoginResult{}, err
	}

	// 用户名不存在和密码错误都返回同一业务错误，避免账号枚举。
	if !security.CheckPassword(command.Password, userRecord.PasswordHash) {
		return LoginResult{}, ErrInvalidCredentials
	}
	if userRecord.Status != userStatusActive {
		return LoginResult{}, ErrAccountDisabled
	}

	// 登录响应需要展示昵称，因此读取与用户一对一的公开资料。
	_, profile, err := s.repo.FindByID(ctx, userRecord.ID)
	if err != nil {
		return LoginResult{}, err
	}

	token, err := s.token.IssueToken(ctx, userRecord.ID)
	if err != nil {
		return LoginResult{}, err
	}

	return LoginResult{
		Token: token,
		User: UserResult{
			ID:       userRecord.ID,
			Username: userRecord.Username,
			Nickname: profile.Nickname,
		},
	}, nil
}

// Me 返回当前登录用户及其公开资料。
func (s *Service) Me(ctx context.Context, userID uint64) (UserResult, error) {
	userRecord, profile, err := s.repo.FindByID(ctx, userID)
	if err != nil {
		return UserResult{}, err
	}
	return UserResult{
		ID:       userRecord.ID,
		Username: userRecord.Username,
		Nickname: profile.Nickname,
	}, nil
}

func validUsername(username string) bool {
	length := utf8.RuneCountInString(username)
	return length >= minUsernameRunes &&
		length <= maxUsernameRunes &&
		usernamePattern.MatchString(username)
}
