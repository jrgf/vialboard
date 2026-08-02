package dto

import "time"

type CreateTeamRequest struct {
	Name      string `json:"name"`
	ManagerID string `json:"managerId"`
}

type UpdateTeamRequest struct {
	ManagerID string `json:"managerId"`
}

type CreateTeamUserRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type TeamResponse struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	ManagerID string    `json:"managerId"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}
