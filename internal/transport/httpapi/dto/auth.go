package dto

import (
	"time"

	"github.com/jrgf/vialboard/internal/domain"
)

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type AuthUserResponse struct {
	ID       string      `json:"id"`
	Username string      `json:"username"`
	Role     domain.Role `json:"role"`
	TeamID   *string     `json:"teamId"`
}

type LoginResponse struct {
	Token     string           `json:"token"`
	ExpiresAt time.Time        `json:"expiresAt"`
	User      AuthUserResponse `json:"user"`
}
