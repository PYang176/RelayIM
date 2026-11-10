package identity

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"relayim/internal/platform/authn"
	"relayim/internal/platform/security"
	"relayim/internal/platform/snowflake"
)

const (
	minPasswordBytes   = 8
	maxPasswordBytes   = 72
	maxNicknameRunes   = 64
	maxBioRunes        = 500
	maxAvatarURLBytes  = 512
	maxDeviceNameRunes = 100
	minUsernameRunes   = 2
	maxUsernameRunes   = 32
)

var usernamePattern = regexp.MustCompile(`^[\p{L}\p{N}_-]+$`)

// TokenIssuer 抽象 Access Token 的签发。测试可以替换为不访问 Redis 的实现。
type TokenIssuer interface {
	IssueToken(ctx context.Context, userID uint64) (string, error)
	IssueRefreshToken(
		ctx context.Context,
	) (string, string, time.Time, error)
	Revoke(
		ctx context.Context,
		token string,
		jti string,
		expiresAt time.Time,
	) error
}

// Service 提供用户注册、登录和当前用户查询。
type Service struct {
	repo    *Repository
	token   TokenIssuer
	avatars AvatarStorage
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

// UpdateProfileCommand 是当前用户修改公开资料的参数。
type UpdateProfileCommand struct {
	// UserID 是当前登录用户 Snowflake ID。
	UserID uint64

	// Nickname 是新的公开昵称，不能为空。
	Nickname string

	// AvatarURL 是可选头像地址；空字符串表示清除头像。
	AvatarURL *string

	// Bio 是可选个人简介；空字符串表示清除简介。
	Bio string
}

// UserResult 是可安全返回给接口层的用户信息。
type UserResult struct {
	// ID 是 Snowflake 用户 ID。
	ID uint64

	// Username 是登录名。
	Username string

	// Nickname 是公开昵称。
	Nickname string

	// AvatarURL 是公开头像地址。
	AvatarURL string

	// Bio 是公开个人简介。
	Bio string
}

// LoginCommand 是用户登录参数。
type LoginCommand struct {
	// Username 是登录名。
	Username string

	// Password 是待校验的明文密码。
	Password string

	// DeviceID 是客户端已保存的设备 Snowflake ID；为零时创建新设备。
	DeviceID uint64

	// DeviceName 是登录设备名称。
	DeviceName string

	// Platform 表示设备平台：1 Web、2 Android、3 iOS、4 Desktop。
	Platform int
}

// LoginResult 是登录成功结果。
type LoginResult struct {
	// Token 是访问受保护接口的 JWT。
	Token string

	// User 是脱敏后的用户信息。
	User UserResult

	// RefreshToken 是用于轮换 Access Token 的长期凭证。
	RefreshToken string

	// RefreshExpiresAt 是 Refresh Token 的 UTC 过期时间。
	RefreshExpiresAt time.Time

	// DeviceID 是本次登录绑定的设备 Snowflake ID。
	DeviceID uint64
}

// RefreshCommand 是轮换 Refresh Token 的参数。
type RefreshCommand struct {
	// RefreshToken 是客户端持有的原始 Refresh Token。
	RefreshToken string
}

// RefreshResult 是 Refresh Token 轮换后的新凭证。
type RefreshResult struct {
	// Token 是新的 Access Token。
	Token string

	// RefreshToken 是新的 Refresh Token；旧值立即失效。
	RefreshToken string

	// RefreshExpiresAt 是新 Refresh Token 的 UTC 过期时间。
	RefreshExpiresAt time.Time
}

// LogoutCommand 是退出登录参数。
type LogoutCommand struct {
	// UserID 是当前登录用户 Snowflake ID。
	UserID uint64

	// AccessToken 是当前请求使用的 JWT。
	AccessToken string

	// JTI 是 Access Token 唯一标识。
	JTI string

	// AccessExpiresAt 是 Access Token 的 UTC 过期时间。
	AccessExpiresAt time.Time

	// RefreshToken 是可选原始 Refresh Token。
	RefreshToken string
}

// EnsureDeviceCommand 是申请 WebSocket Ticket 时的设备信息。
type EnsureDeviceCommand struct {
	// UserID 是当前登录用户 Snowflake ID。
	UserID uint64

	// DeviceID 是客户端已保存的设备 Snowflake ID；为空时创建新设备。
	DeviceID uint64

	// DeviceName 是客户端展示名称。
	DeviceName string

	// Platform 表示设备平台：1 Web、2 Android、3 iOS、4 Desktop。
	Platform int
}

// DeviceResult 是设备注册结果。
type DeviceResult struct {
	// ID 是设备 Snowflake ID。
	ID uint64

	// DeviceName 是设备展示名称。
	DeviceName string

	// Platform 表示设备平台。
	Platform int
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
		ID:        userRecord.ID,
		Username:  userRecord.Username,
		Nickname:  profile.Nickname,
		AvatarURL: stringValue(profile.AvatarURL),
		Bio:       stringValue(profile.Bio),
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

	device, err := s.EnsureDevice(ctx, EnsureDeviceCommand{
		UserID:     userRecord.ID,
		DeviceID:   command.DeviceID,
		DeviceName: command.DeviceName,
		Platform:   command.Platform,
	})
	if errors.Is(err, ErrDeviceNotFound) && command.DeviceID > 0 {
		/*
		 * 浏览器切换账号时可能保留上一个账号的设备 ID。登录阶段
		 * 应创建当前账号的新设备，而不是让用户手工清理本地存储。
		 * WebSocket Ticket 阶段仍会严格拒绝不属于当前用户的设备。
		 */
		device, err = s.EnsureDevice(ctx, EnsureDeviceCommand{
			UserID:     userRecord.ID,
			DeviceName: command.DeviceName,
			Platform:   command.Platform,
		})
	}
	if err != nil {
		return LoginResult{}, err
	}
	rawRefreshToken, refreshHash, refreshExpiresAt, err :=
		s.token.IssueRefreshToken(ctx)
	if err != nil {
		return LoginResult{}, err
	}
	now := time.Now().UTC()
	if err := s.repo.createRefreshToken(ctx, &refreshToken{
		ID:        uint64(snowflake.Generate()),
		UserID:    userRecord.ID,
		DeviceID:  device.ID,
		TokenHash: refreshHash,
		ExpiresAt: refreshExpiresAt,
		CreatedAt: now,
	}); err != nil {
		return LoginResult{}, err
	}
	token, err := s.token.IssueToken(ctx, userRecord.ID)
	if err != nil {
		_ = s.repo.revokeRefreshToken(
			context.WithoutCancel(ctx),
			refreshHash,
			time.Now().UTC(),
		)
		return LoginResult{}, err
	}

	return LoginResult{
		Token:            token,
		RefreshToken:     rawRefreshToken,
		RefreshExpiresAt: refreshExpiresAt,
		DeviceID:         device.ID,
		User: UserResult{
			ID:        userRecord.ID,
			Username:  userRecord.Username,
			Nickname:  profile.Nickname,
			AvatarURL: stringValue(profile.AvatarURL),
			Bio:       stringValue(profile.Bio),
		},
	}, nil
}

// Refresh 原子轮换 Refresh Token 并签发新的 Access Token。
func (s *Service) Refresh(
	ctx context.Context,
	command RefreshCommand,
) (RefreshResult, error) {
	command.RefreshToken = strings.TrimSpace(command.RefreshToken)
	if command.RefreshToken == "" {
		return RefreshResult{}, ErrInvalidRefreshToken
	}

	newRaw, newHash, expiresAt, err := s.token.IssueRefreshToken(ctx)
	if err != nil {
		return RefreshResult{}, err
	}
	now := time.Now().UTC()
	userID, _, err := s.repo.rotateRefreshToken(
		ctx,
		authn.HashRefreshToken(command.RefreshToken),
		&refreshToken{
			ID:        uint64(snowflake.Generate()),
			TokenHash: newHash,
			ExpiresAt: expiresAt,
			CreatedAt: now,
		},
		now,
	)
	if err != nil {
		return RefreshResult{}, err
	}
	accessToken, err := s.token.IssueToken(ctx, userID)
	if err != nil {
		_ = s.repo.revokeRefreshToken(
			context.WithoutCancel(ctx),
			newHash,
			time.Now().UTC(),
		)
		return RefreshResult{}, err
	}
	return RefreshResult{
		Token:            accessToken,
		RefreshToken:     newRaw,
		RefreshExpiresAt: expiresAt,
	}, nil
}

// Logout 撤销 Access Token 和可选的 Refresh Token。
func (s *Service) Logout(
	ctx context.Context,
	command LogoutCommand,
) error {
	if command.UserID == 0 || command.AccessToken == "" || command.JTI == "" {
		return ErrInvalidInput
	}
	accessErr := s.token.Revoke(
		ctx,
		command.AccessToken,
		command.JTI,
		command.AccessExpiresAt,
	)
	var refreshErr error
	if strings.TrimSpace(command.RefreshToken) != "" {
		refreshErr = s.repo.revokeRefreshToken(
			ctx,
			authn.HashRefreshToken(command.RefreshToken),
			time.Now().UTC(),
		)
	}
	return errors.Join(accessErr, refreshErr)
}

// Me 返回当前登录用户及其公开资料。
func (s *Service) Me(ctx context.Context, userID uint64) (UserResult, error) {
	userRecord, profile, err := s.repo.FindByID(ctx, userID)
	if err != nil {
		return UserResult{}, err
	}
	return UserResult{
		ID:        userRecord.ID,
		Username:  userRecord.Username,
		Nickname:  profile.Nickname,
		AvatarURL: stringValue(profile.AvatarURL),
		Bio:       stringValue(profile.Bio),
	}, nil
}

// UpdateProfile 校验并更新当前用户的昵称、头像和个人简介。
func (s *Service) UpdateProfile(
	ctx context.Context,
	command UpdateProfileCommand,
) (UserResult, error) {
	command.Nickname = strings.TrimSpace(command.Nickname)
	command.Bio = strings.TrimSpace(command.Bio)
	if command.AvatarURL != nil {
		avatarURL := strings.TrimSpace(*command.AvatarURL)
		if !validAvatarURL(avatarURL) {
			return UserResult{}, ErrInvalidInput
		}
		command.AvatarURL = &avatarURL
	}
	if command.UserID == 0 ||
		command.Nickname == "" ||
		utf8.RuneCountInString(command.Nickname) > maxNicknameRunes ||
		utf8.RuneCountInString(command.Bio) > maxBioRunes {
		return UserResult{}, ErrInvalidInput
	}

	if err := s.repo.UpdateProfile(
		ctx,
		command.UserID,
		command.Nickname,
		command.AvatarURL,
		command.Bio,
		time.Now().UTC(),
	); err != nil {
		return UserResult{}, err
	}
	userRecord, profile, err := s.repo.FindByID(ctx, command.UserID)
	if err != nil {
		return UserResult{}, err
	}
	return UserResult{
		ID:        userRecord.ID,
		Username:  userRecord.Username,
		Nickname:  profile.Nickname,
		AvatarURL: stringValue(profile.AvatarURL),
		Bio:       stringValue(profile.Bio),
	}, nil
}

// EnsureDevice 查询或创建设备，并更新最近活跃时间。
func (s *Service) EnsureDevice(
	ctx context.Context,
	command EnsureDeviceCommand,
) (DeviceResult, error) {
	if command.UserID == 0 {
		return DeviceResult{}, ErrInvalidInput
	}

	now := time.Now().UTC()
	if command.DeviceID > 0 {
		record, err := s.repo.FindActiveDevice(
			ctx,
			command.UserID,
			command.DeviceID,
		)
		if err != nil {
			return DeviceResult{}, err
		}
		if err := s.repo.TouchDevice(ctx, record.ID, now); err != nil {
			return DeviceResult{}, err
		}
		return DeviceResult{
			ID:         record.ID,
			DeviceName: record.DeviceName,
			Platform:   record.Platform,
		}, nil
	}

	command.DeviceName = strings.TrimSpace(command.DeviceName)
	if command.DeviceName == "" {
		command.DeviceName = "Web"
	}
	if command.Platform == 0 {
		command.Platform = 1
	}
	if command.Platform < 1 ||
		command.Platform > 4 ||
		utf8.RuneCountInString(command.DeviceName) > maxDeviceNameRunes {
		return DeviceResult{}, ErrInvalidInput
	}

	record := &device{
		ID:         uint64(snowflake.Generate()),
		UserID:     command.UserID,
		Platform:   command.Platform,
		DeviceName: command.DeviceName,
		LastSeenAt: &now,
		CreatedAt:  now,
	}
	if err := s.repo.CreateDevice(ctx, record); err != nil {
		return DeviceResult{}, err
	}
	return DeviceResult{
		ID:         record.ID,
		DeviceName: record.DeviceName,
		Platform:   record.Platform,
	}, nil
}

// validUsername 校验用户名长度、字符集和空白边界
func validUsername(username string) bool {
	length := utf8.RuneCountInString(username)
	return length >= minUsernameRunes &&
		length <= maxUsernameRunes &&
		usernamePattern.MatchString(username)
}

// validAvatarURL 校验头像 URL 只允许受支持的本地资源路径
func validAvatarURL(value string) bool {
	if value == "" {
		return true
	}
	if len(value) > maxAvatarURLBytes {
		return false
	}
	parsed, err := url.ParseRequestURI(value)
	if err != nil || parsed.Host == "" {
		return false
	}
	return parsed.Scheme == "http" || parsed.Scheme == "https"
}

// stringValue 读取字符串值并去除首尾空白
func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
