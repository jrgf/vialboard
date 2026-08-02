package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	vial "github.com/jrgf/go-vial"
	"github.com/jrgf/vialboard/internal/application"
	"github.com/jrgf/vialboard/internal/domain"
	"github.com/jrgf/vialboard/internal/transport/httpapi/dto"
)

type notificationsAPI struct {
	notifications *application.NotificationService
	users         *application.UserService
	hub           *notificationHub
}

func (api notificationsAPI) register(app *vial.App, authenticated vial.Middleware) {
	app.Get("/notifications", authenticated(api.list))
	app.Get("/notifications/stream", authenticated(api.stream))
	app.Patch("/notifications/{id}/read", authenticated(api.markRead))
	app.Post("/notifications/readAll", authenticated(api.markAllRead))
}

func (api notificationsAPI) list(c *vial.Context) error {
	actor, err := currentUser(c)
	if err != nil {
		return err
	}
	inbox, err := api.notifications.List(c.Request().Context(), actor.ID)
	if err != nil {
		return err
	}
	items := make([]dto.NotificationResponse, len(inbox.Items))
	for index, notification := range inbox.Items {
		items[index] = toNotificationResponse(notification)
	}
	return c.JSON(http.StatusOK, dto.NotificationListResponse{Items: items, Unread: inbox.Unread})
}

func (api notificationsAPI) markRead(c *vial.Context) error {
	actor, err := currentUser(c)
	if err != nil {
		return err
	}
	id, err := notificationID(c)
	if err != nil {
		return err
	}
	if err := api.notifications.MarkRead(c.Request().Context(), actor.ID, id); err != nil {
		if errors.Is(err, domain.ErrNotificationNotFound) {
			return vial.NewHTTPError(http.StatusNotFound, "notificationNotFound", "Notification was not found")
		}
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

func (api notificationsAPI) markAllRead(c *vial.Context) error {
	actor, err := currentUser(c)
	if err != nil {
		return err
	}
	if err := api.notifications.MarkAllRead(c.Request().Context(), actor.ID); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

func (api notificationsAPI) stream(c *vial.Context) error {
	actor, err := currentUser(c)
	if err != nil {
		return err
	}
	parts := strings.Fields(c.Request().Header.Get("Authorization"))
	token := parts[1]
	stream, unsubscribe := api.hub.subscribe(actor.ID)
	defer unsubscribe()

	response := c.Response()
	if err := disableWriteDeadline(response); err != nil {
		return fmt.Errorf("disable notification stream write deadline: %w", err)
	}
	response.Header().Set("Content-Type", "text/event-stream")
	response.Header().Set("Cache-Control", "no-cache, no-store")
	response.Header().Set("Connection", "keep-alive")
	response.Header().Set("X-Accel-Buffering", "no")
	if _, err := fmt.Fprint(response, "retry: 2000\n\n"); err != nil {
		return nil
	}
	response.(http.Flusher).Flush()

	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case <-c.Request().Context().Done():
			return nil
		case notification := <-stream:
			if !api.sessionActive(c, token) {
				return nil
			}
			payload, err := json.Marshal(toNotificationResponse(notification))
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintf(response, "data: %s\n\n", payload); err != nil {
				return nil
			}
			response.(http.Flusher).Flush()
		case <-heartbeat.C:
			if !api.sessionActive(c, token) {
				return nil
			}
			if _, err := fmt.Fprint(response, ": heartbeat\n\n"); err != nil {
				return nil
			}
			response.(http.Flusher).Flush()
		}
	}
}

func disableWriteDeadline(response http.ResponseWriter) error {
	return http.NewResponseController(response).SetWriteDeadline(time.Time{})
}

func (api notificationsAPI) sessionActive(c *vial.Context, token string) bool {
	_, err := api.users.Authenticate(c.Request().Context(), token)
	return err == nil
}

func notificationID(c *vial.Context) (uint64, error) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		return 0, vial.NewHTTPError(http.StatusBadRequest, "invalidNotificationId", "Notification ID must be a positive integer")
	}
	return id, nil
}

func toNotificationResponse(notification domain.Notification) dto.NotificationResponse {
	return dto.NotificationResponse{
		ID: notification.ID, Kind: notification.Kind, Message: notification.Message,
		IssueID: notification.IssueID, TeamID: notification.TeamID, ReadAt: notification.ReadAt, CreatedAt: notification.CreatedAt,
	}
}
