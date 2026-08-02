package httpapi

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/jrgf/vialboard/internal/application"
	"github.com/jrgf/vialboard/internal/domain"
)

type notificationHub struct {
	mu          sync.RWMutex
	subscribers map[string]map[chan domain.Notification]struct{}
}

func newNotificationHub() *notificationHub {
	return &notificationHub{subscribers: make(map[string]map[chan domain.Notification]struct{})}
}

func (hub *notificationHub) subscribe(userID string) (<-chan domain.Notification, func()) {
	stream := make(chan domain.Notification, 16)
	hub.mu.Lock()
	if hub.subscribers[userID] == nil {
		hub.subscribers[userID] = make(map[chan domain.Notification]struct{})
	}
	hub.subscribers[userID][stream] = struct{}{}
	hub.mu.Unlock()

	return stream, func() {
		hub.mu.Lock()
		if subscribers := hub.subscribers[userID]; subscribers != nil {
			delete(subscribers, stream)
			if len(subscribers) == 0 {
				delete(hub.subscribers, userID)
			}
		}
		hub.mu.Unlock()
	}
}

func (hub *notificationHub) publish(notification domain.Notification) {
	hub.mu.RLock()
	defer hub.mu.RUnlock()
	for subscriber := range hub.subscribers[notification.UserID] {
		select {
		case subscriber <- notification:
		default:
			// The durable inbox lets a slow client recover without blocking every subscriber.
		}
	}
}

func notificationBroadcaster(notifications *application.NotificationService, hub *notificationHub) func(context.Context) error {
	return func(ctx context.Context) error {
		latestID, err := notifications.LatestID(ctx)
		if err != nil {
			return err
		}
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return nil
			case <-ticker.C:
				items, err := notifications.ListAfter(ctx, latestID)
				if err != nil {
					slog.ErrorContext(ctx, "poll notifications", "error", err)
					continue
				}
				for _, notification := range items {
					hub.publish(notification)
					latestID = notification.ID
				}
			}
		}
	}
}
