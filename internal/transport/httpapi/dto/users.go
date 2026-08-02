package dto

import (
	"time"

	"github.com/jrgf/vialboard/internal/domain"
)

type RegisterUserRequest struct {
	Username             string `json:"username"`
	Password             string `json:"password"`
	PasswordConfirmation string `json:"passwordConfirmation"`
}

type CreateUserRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Role     string `json:"role"`
}

type UpdateUserAccessRequest struct {
	Role   *string `json:"role"`
	Active *bool   `json:"active"`
}

type ChangePasswordRequest struct {
	CurrentPassword      string `json:"currentPassword"`
	NewPassword          string `json:"newPassword"`
	PasswordConfirmation string `json:"passwordConfirmation"`
}

type ResetPasswordRequest struct {
	NewPassword          string `json:"newPassword"`
	PasswordConfirmation string `json:"passwordConfirmation"`
}

type ListUsersQuery struct {
	Page     int `query:"page"`
	PageSize int `query:"pageSize"`
}

type UserResponse struct {
	ID        string      `json:"id"`
	Username  string      `json:"username"`
	Role      domain.Role `json:"role"`
	TeamID    *string     `json:"teamId"`
	Active    bool        `json:"active"`
	CreatedAt time.Time   `json:"createdAt"`
	UpdatedAt time.Time   `json:"updatedAt"`
}

type UserListResponse struct {
	Items      []UserResponse     `json:"items"`
	Pagination PaginationResponse `json:"pagination"`
}

type MemberResponse struct {
	ID       string      `json:"id"`
	Username string      `json:"username"`
	Role     domain.Role `json:"role"`
	TeamID   *string     `json:"teamId"`
}
