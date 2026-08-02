package httpapi

import (
	"errors"
	"net/http"
	"strings"

	vial "github.com/jrgf/go-vial"
	"github.com/jrgf/vialboard/internal/application"
	"github.com/jrgf/vialboard/internal/domain"
)

const (
	currentUserKey    = "currentUser"
	currentSessionKey = "currentSession"
)

func authenticate(users *application.UserService) vial.Middleware {
	return func(next vial.Handler) vial.Handler {
		return func(c *vial.Context) error {
			parts := strings.Fields(c.Request().Header.Get("Authorization"))
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				return unauthorized(c)
			}
			user, err := users.Authenticate(c.Request().Context(), parts[1])
			if errors.Is(err, application.ErrInvalidSession) {
				return unauthorized(c)
			}
			if err != nil {
				return err
			}
			c.Set(currentUserKey, user)
			c.Set(currentSessionKey, domain.SessionID(parts[1]))
			return next(c)
		}
	}
}

func requireRole(role domain.Role) vial.Middleware {
	return func(next vial.Handler) vial.Handler {
		return func(c *vial.Context) error {
			user, err := currentUser(c)
			if err != nil || user.Role != role {
				return vial.NewHTTPError(http.StatusForbidden, "forbidden", "You do not have permission to perform this action")
			}
			return next(c)
		}
	}
}

func currentUser(c *vial.Context) (domain.User, error) {
	value, ok := c.Get(currentUserKey)
	user, valid := value.(domain.User)
	if !ok || !valid {
		return domain.User{}, unauthorized(c)
	}
	return user, nil
}

func unauthorized(c *vial.Context) error {
	c.Response().Header().Set("WWW-Authenticate", "Bearer")
	return vial.NewHTTPError(http.StatusUnauthorized, "unauthorized", "Authentication is required")
}
