package dto

import (
	"time"

	"github.com/jrgf/vialboard/internal/domain"
)

type CreateIssueRequest struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Status      string `json:"status"`
	Priority    string `json:"priority"`
	DueDate     string `json:"dueDate"`
	TeamID      string `json:"teamId"`
	AssigneeID  string `json:"assigneeId"`
}

type UpdateIssueRequest struct {
	Title       *string `json:"title"`
	Description *string `json:"description"`
	Status      *string `json:"status"`
	Priority    *string `json:"priority"`
	DueDate     *string `json:"dueDate"`
	TeamID      *string `json:"teamId"`
	AssigneeID  *string `json:"assigneeId"`
}

type ListIssuesQuery struct {
	Status   string `query:"status"`
	Priority string `query:"priority"`
	TeamID   string `query:"teamId"`
	Search   string `query:"search"`
	Sort     string `query:"sort"`
	Order    string `query:"order"`
	Page     int    `query:"page"`
	PageSize int    `query:"pageSize"`
}

type IssueResponse struct {
	ID          uint64          `json:"id"`
	Title       string          `json:"title"`
	Description string          `json:"description"`
	Status      domain.Status   `json:"status"`
	Priority    domain.Priority `json:"priority"`
	DueDate     *string         `json:"dueDate"`
	CreatedBy   string          `json:"createdBy"`
	TeamID      *string         `json:"teamId"`
	AssigneeID  *string         `json:"assigneeId"`
	CreatedAt   time.Time       `json:"createdAt"`
	UpdatedAt   time.Time       `json:"updatedAt"`
}

type IssueListResponse struct {
	Items      []IssueResponse    `json:"items"`
	Pagination PaginationResponse `json:"pagination"`
}

type CreateCommentRequest struct {
	Body string `json:"body"`
}

type ListActivityQuery struct {
	Page     int `query:"page"`
	PageSize int `query:"pageSize"`
}

type ActivityResponse struct {
	ID            uint64              `json:"id"`
	IssueID       uint64              `json:"issueId"`
	ActorID       string              `json:"actorId"`
	ActorUsername string              `json:"actorUsername"`
	Kind          domain.ActivityKind `json:"kind"`
	Body          string              `json:"body"`
	CreatedAt     time.Time           `json:"createdAt"`
}

type ActivityListResponse struct {
	Items      []ActivityResponse `json:"items"`
	Pagination PaginationResponse `json:"pagination"`
}
