package application

import (
	"context"
	"time"

	"github.com/jrgf/vialboard/internal/domain"
)

const notificationListLimit = 50

type NotificationRepository interface {
	CreateMany(context.Context, []domain.Notification) error
	List(context.Context, string, int) ([]domain.Notification, int64, error)
	MarkRead(context.Context, string, uint64, time.Time) error
	MarkAllRead(context.Context, string, time.Time) error
	LatestID(context.Context) (uint64, error)
	ListAfter(context.Context, uint64, int) ([]domain.Notification, error)
}

type NotificationInbox struct {
	Items  []domain.Notification
	Unread int64
}

type NotificationService struct {
	repository NotificationRepository
}

func NewNotificationService(repository NotificationRepository) *NotificationService {
	return &NotificationService{repository: repository}
}

func (s *NotificationService) Notify(ctx context.Context, recipients []string, actorID string, kind domain.NotificationKind, message string, issueID *uint64, teamID *string) error {
	seen := make(map[string]struct{}, len(recipients))
	notifications := make([]domain.Notification, 0, len(recipients))
	for _, userID := range recipients {
		if userID == actorID {
			continue
		}
		if _, exists := seen[userID]; exists {
			continue
		}
		notification, err := domain.NewNotification(userID, kind, message, issueID, teamID)
		if err != nil {
			return err
		}
		seen[userID] = struct{}{}
		notifications = append(notifications, notification)
	}
	if len(notifications) == 0 {
		return nil
	}
	return s.repository.CreateMany(ctx, notifications)
}

func (s *NotificationService) List(ctx context.Context, userID string) (NotificationInbox, error) {
	items, unread, err := s.repository.List(ctx, userID, notificationListLimit)
	return NotificationInbox{Items: items, Unread: unread}, err
}

func (s *NotificationService) MarkRead(ctx context.Context, userID string, id uint64) error {
	return s.repository.MarkRead(ctx, userID, id, time.Now().UTC())
}

func (s *NotificationService) MarkAllRead(ctx context.Context, userID string) error {
	return s.repository.MarkAllRead(ctx, userID, time.Now().UTC())
}

func (s *NotificationService) LatestID(ctx context.Context) (uint64, error) {
	return s.repository.LatestID(ctx)
}

func (s *NotificationService) ListAfter(ctx context.Context, id uint64) ([]domain.Notification, error) {
	return s.repository.ListAfter(ctx, id, 100)
}
