package identity

import "errors"

var (
	// ErrInvalidInput 表示注册或登录参数不符合业务规则。
	ErrInvalidInput = errors.New("认证参数不合法")

	// ErrUsernameExists 表示用户名已被占用。
	ErrUsernameExists = errors.New("用户名已存在")

	// ErrInvalidCredentials 表示用户名不存在或密码错误。
	// 对外统一使用该错误，避免泄露账号是否存在。
	ErrInvalidCredentials = errors.New("用户名或密码错误")

	// ErrAccountDisabled 表示账号存在但已被禁用。
	ErrAccountDisabled = errors.New("账号已被禁用")

	// ErrUserNotFound 表示按用户 ID 查询时用户不存在。
	ErrUserNotFound = errors.New("用户不存在")

	// ErrDeviceNotFound 表示设备不存在、已撤销或不属于当前用户。
	ErrDeviceNotFound = errors.New("设备不存在或无权访问")

	// ErrInvalidRefreshToken 表示 Refresh Token 不存在、过期或已撤销。
	ErrInvalidRefreshToken = errors.New("Refresh Token 无效或已过期")

	// ErrInvalidAvatar 表示头像文件类型、大小或内容不合法。
	ErrInvalidAvatar = errors.New("头像图片不合法")

	// ErrAvatarNotFound 表示用户没有已上传的头像。
	ErrAvatarNotFound = errors.New("头像不存在")

	// ErrAvatarStorageUnavailable 表示头像对象存储未配置或暂时不可用。
	ErrAvatarStorageUnavailable = errors.New("头像存储服务暂不可用")
)
