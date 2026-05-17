package service

import (
	"context"
	"errors"
	"fmt"
	"ginChatProject/common/validator"
	"ginChatProject/internal/dto"
	"ginChatProject/internal/model"
	"ginChatProject/internal/repository"
	"ginChatProject/utils/jwt"
	"ginChatProject/utils/redis_util"
	"ginChatProject/utils/security"
	"gorm.io/gorm"
	"time"
)

type UserDeviceInfo struct {
	//客户端ip
	ClientIp string
	//客户端端口
	ClientPort string
	//登录时间
	LoginTime *time.Time
	//设备信息
	DeviceInfo string
}

type UserService struct {
	repo *repository.UserRepository
}

func NewUserService(repo *repository.UserRepository) *UserService {
	return &UserService{repo: repo}
}

// GetUserList 获取用户列表
func (s *UserService) GetUserList(ctx context.Context) ([]*model.UserBasic, error) {
	return s.repo.List(ctx)
}

// CreateUser 创建用户
func (s *UserService) CreateUser(ctx context.Context, req *dto.UserCreateDTO) error {
	// 检测重复密码是否一致
	if req.Password != req.ConfirmPassword {
		return errors.New("两次输入的密码不一致")
	}

	// 检测用户名是否重复
	exists, err := s.repo.ExistsByName(ctx, req.Name)
	if err != nil {
		return err
	}
	if exists {
		return errors.New("该用户名已存在")
	}

	// 进行密码加密
	hashPwd, err := security.HashPassword(req.Password)
	if err != nil {
		return err
	}

	// 映射DTO到Model
	user := model.UserBasic{
		Name:     req.Name,
		Password: hashPwd,
	}

	return s.repo.Create(ctx, &user)
}

// DeleteUser 删除用户
func (s *UserService) DeleteUser(ctx context.Context, req *dto.UserDeleteDTO) error {
	user := model.UserBasic{
		Model: gorm.Model{ID: req.Id},
	}
	return s.repo.Delete(ctx, &user)
}

// UpdateUser 修改用户
func (s *UserService) UpdateUser(ctx context.Context, req dto.UserUpdateDTO) error {
	// 1. 校验参数
	if err := validator.ValidateStruct(req); err != nil {
		return err
	}

	// 2. 判断用户是否存在
	_, err := s.repo.FindByID(ctx, req.ID)
	if err != nil {
		return errors.New("用户不存在")
	}

	// 3. 构造更新字段
	//updates := repository.UserUpdateFields{}
	//if req.Name != "" {
	//	updates.Name = &req.Name
	//}
	//if req.Phone != "" {
	//	updates.Phone = &req.Phone
	//}
	//if req.Email != "" {
	//	updates.Email = &req.Email
	//}
	updates := model.UserBasic{}
	if req.Name != "" {
		updates.Name = req.Name
	}
	if req.Phone != "" {
		updates.Phone = req.Phone
	}
	if req.Email != "" {
		updates.Email = req.Email
	}
	return s.repo.Update(ctx, req.ID, &updates)
}

// Login 登录方法
func (s *UserService) Login(ctx context.Context, req dto.UserLoginDTO, deviceInfo UserDeviceInfo) (string, error) {

	// 1. 查用户
	user, err := s.repo.FindByName(ctx, req.Name)
	if err != nil {
		return "", errors.New("用户不存在")
	}

	// 2. 校验密码
	if !security.CheckPassword(req.Password, user.Password) {
		return "", errors.New("用户名或密码错误")
	}

	// 3. 生成 Token
	token, err := jwt.GenerateToken(user.ID, jwt.JwtCfg.ExpireHours)
	if err != nil {
		return "", err
	}

	// 更新数据库登录数据
	{
		user.LoginTime = deviceInfo.LoginTime
		user.ClientIp = deviceInfo.ClientIp
		user.ClientPort = deviceInfo.ClientPort
		user.DeviceInfo = deviceInfo.DeviceInfo
		user.IsLogout = false
		user.LogoutTime = nil
		user.HeartbeatTime = nil
	}

	//  关键：保存到数据库
	if err := s.repo.Update(ctx, user.ID, user); err != nil {
		return "", errors.New("更新登录信息失败")
	}

	// 4. 存入 Redis（key = token，value = userID）
	key := fmt.Sprintf("token:%s", token)
	err = redis_util.RDB.Set(
		//redis_util.Ctx,
		ctx,
		key,
		user.ID,
		time.Duration(jwt.JwtCfg.ExpireHours)*time.Hour,
	).Err()

	return token, nil
}

// Logout 退出登录
func (s *UserService) Logout(ctx context.Context, userID uint, logoutTime time.Time) error {
	user, err := s.repo.FindByID(ctx, userID)
	if err != nil {
		return err
	}

	// 更新登出信息
	user.LogoutTime = &logoutTime
	user.IsLogout = true
	user.HeartbeatTime = nil // 清空心跳时间

	return s.repo.Update(ctx, userID, user)

}
