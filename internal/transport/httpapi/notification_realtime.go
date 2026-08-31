package httpapi

import (
	"context"
	"log/slog"
	"time"

	"github.com/jrgf/go-vial/sse"
	"github.com/jrgf/vialboard/internal/application"
)

func notificationBroadcaster(notifications *application.NotificationService, hub *sse.Hub) func(context.Context) error {
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
					latestID = notification.ID
					event, err := sse.JSON("", toNotificationResponse(notification))
					if err != nil {
						slog.ErrorContext(ctx, "encode notification event", "error", err)
						continue
					}
					if _, err := hub.PublishTopic(notification.UserID, event); err != nil {
						slog.ErrorContext(ctx, "publish notification event", "error", err)
					}
				}
			}
		}
	}
}
