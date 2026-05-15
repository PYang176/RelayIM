package dto

// FriendRequestDTO 好友申请DTO
type FriendRequestDTO struct {
	ToUserID uint64 `json:"to_user_id" binding:"required"`
	Remark   string `json:"remark,omitempty"`
}
