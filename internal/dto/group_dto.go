package dto

type CreateGroupDTO struct {
	Name   string `json:"name" binding:"required"`
	Avatar string `json:"avatar"`
}
