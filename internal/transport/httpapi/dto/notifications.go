package dto

import (
	"time"

	"github.com/jrgf/vialboard/internal/domain"
)

type NotificationResponse struct {
	ID        uint64                  `json:"id"`
	Kind      domain.NotificationKind `json:"kind"`
	Message   string                  `json:"message"`
	IssueID   *uint64                 `json:"issueId"`
	TeamID    *string                 `json:"teamId"`
	ReadAt    *time.Time              `json:"readAt"`
	CreatedAt time.Time               `json:"createdAt"`
}

type NotificationListResponse struct {
	Items  []NotificationResponse `json:"items"`
	Unread int64                  `json:"unread"`
}
