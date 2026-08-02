package httpapi

import (
	"testing"
	"time"

	"github.com/jrgf/vialboard/internal/domain"
)

func TestNotificationHubRoutesByUser(t *testing.T) {
	hub := newNotificationHub()
	first, unsubscribeFirst := hub.subscribe("first")
	defer unsubscribeFirst()
	second, unsubscribeSecond := hub.subscribe("second")
	defer unsubscribeSecond()

	hub.publish(domain.Notification{ID: 7, UserID: "first"})
	select {
	case notification := <-first:
		if notification.ID != 7 {
			t.Fatalf("notification ID = %d", notification.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("target subscriber did not receive notification")
	}
	select {
	case <-second:
		t.Fatal("notification leaked to another user")
	default:
	}
}
