package dto

// UserCreateDTO 注册用户
type UserCreateDTO struct {
	Name            string `json:"name" validate:"required,min=2,max=20"`
	Password        string `json:"password" validate:"required,min=6"`
	ConfirmPassword string `json:"confirm_password" validate:"required,eqfield=Password"`
}

// UserDeleteDTO 删除用户(只做逻辑删除)
type UserDeleteDTO struct {
	Id uint `json:"id"`
}

// UserUpdateDTO 修改用户
type UserUpdateDTO struct {
	ID    uint   `json:"-"`
	Name  string `json:"name,omitempty" validate:"omitempty,min=2,max=20"`
	Phone string `json:"phone,omitempty" validate:"omitempty,e164"`
	Email string `json:"email,omitempty" validate:"omitempty,email"`
}

// UserLoginDTO 用户登录
type UserLoginDTO struct {
	Name     string `json:"name" validate:"required"`
	Password string `json:"password" validate:"required"`
}
