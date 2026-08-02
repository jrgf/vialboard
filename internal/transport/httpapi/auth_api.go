package httpapi

import (
	"errors"
	"net/http"

	vial "github.com/jrgf/go-vial"
	"github.com/jrgf/vialboard/internal/application"
	"github.com/jrgf/vialboard/internal/domain"
	"github.com/jrgf/vialboard/internal/transport/httpapi/dto"
)

type authAPI struct {
	users *application.UserService
}

func (api authAPI) register(app *vial.App, authenticated, limited vial.Middleware) {
	app.Post("/login", limited(api.login))
	app.Post("/logout", authenticated(api.logout))
}

func (api authAPI) login(c *vial.Context) error {
	var request dto.LoginRequest
	if err := c.BindJSON(&request); err != nil {
		return err
	}
	c.Response().Header().Set("Cache-Control", "no-store")
	user, session, err := api.users.Login(c.Request().Context(), request.Username, request.Password)
	if errors.Is(err, application.ErrInvalidCredentials) {
		return vial.NewHTTPError(http.StatusUnauthorized, "invalidCredentials", "Username or password is incorrect")
	}
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, toLoginResponse(user, session))
}

func toLoginResponse(user domain.User, session domain.Session) dto.LoginResponse {
	return dto.LoginResponse{
		Token:     session.Token,
		ExpiresAt: session.ExpiresAt,
		User: dto.AuthUserResponse{
			ID:       user.ID,
			Username: user.Username,
			Role:     user.Role,
			TeamID:   user.TeamID,
		},
	}
}

func (api authAPI) logout(c *vial.Context) error {
	value, _ := c.Get(currentSessionKey)
	sessionID, _ := value.(string)
	if err := api.users.Logout(c.Request().Context(), sessionID); err != nil {
		if errors.Is(err, application.ErrInvalidSession) {
			return unauthorized(c)
		}
		return err
	}
	return c.NoContent(http.StatusNoContent)
}
