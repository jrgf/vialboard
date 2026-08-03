package httpapi

import (
	"errors"
	"net/http"

	vial "github.com/jrgf/go-vial"
	"github.com/jrgf/vialboard/internal/application"
	"github.com/jrgf/vialboard/internal/domain"
	"github.com/jrgf/vialboard/internal/transport/httpapi/dto"
)

type usersAPI struct {
	users *application.UserService
}

func (api usersAPI) register(public, protected, admin *vial.Group, limited vial.Middleware) {
	public.Post("/register", limited(api.createViewer))
	protected.Get("/members", api.members)
	protected.Patch("/account/password", api.changePassword)
	admin.Post("/users", api.create)
	admin.Get("/users", api.list)
	admin.Patch("/users/{id}", api.updateAccess)
	admin.Patch("/users/{id}/password", api.resetPassword)
}

func (api usersAPI) changePassword(c *vial.Context) error {
	actor, err := currentUser(c)
	if err != nil {
		return err
	}
	var request dto.ChangePasswordRequest
	if err := c.BindJSON(&request); err != nil {
		return err
	}
	if err := confirmPassword(request.NewPassword, request.PasswordConfirmation); err != nil {
		return userTransportError(err)
	}
	if err := api.users.ChangePassword(c.Request().Context(), actor, request.CurrentPassword, request.NewPassword); err != nil {
		return userTransportError(err)
	}
	return c.NoContent(http.StatusNoContent)
}

func (api usersAPI) resetPassword(c *vial.Context) error {
	id, err := userID(c)
	if err != nil {
		return err
	}
	var request dto.ResetPasswordRequest
	if err := c.BindJSON(&request); err != nil {
		return err
	}
	if err := confirmPassword(request.NewPassword, request.PasswordConfirmation); err != nil {
		return userTransportError(err)
	}
	if err := api.users.ResetPassword(c.Request().Context(), id, request.NewPassword); err != nil {
		return userTransportError(err)
	}
	return c.NoContent(http.StatusNoContent)
}

func (api usersAPI) members(c *vial.Context) error {
	actor, err := currentUser(c)
	if err != nil {
		return err
	}
	users, err := api.users.Members(c.Request().Context(), actor)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, toMemberResponses(users))
}

func (api usersAPI) createViewer(c *vial.Context) error {
	var request dto.RegisterUserRequest
	if err := c.BindJSON(&request); err != nil {
		return err
	}
	if err := confirmPassword(request.Password, request.PasswordConfirmation); err != nil {
		return userTransportError(err)
	}
	c.Response().Header().Set("Cache-Control", "no-store")
	user, session, err := api.users.Register(c.Request().Context(), request.Username, request.Password)
	if err != nil {
		return userTransportError(err)
	}
	return c.JSON(http.StatusCreated, toLoginResponse(user, session))
}

func confirmPassword(password, confirmation string) error {
	if password != confirmation {
		return domain.NewValidationError("passwordMismatch", "Passwords do not match")
	}
	return nil
}

func (api usersAPI) create(c *vial.Context) error {
	var request dto.CreateUserRequest
	if err := c.BindJSON(&request); err != nil {
		return err
	}
	user, err := api.users.Create(c.Request().Context(), request.Username, request.Password, request.Role)
	if err != nil {
		return userTransportError(err)
	}
	return c.JSON(http.StatusCreated, toUserResponse(user))
}

func (api usersAPI) list(c *vial.Context) error {
	var query dto.ListUsersQuery
	if err := c.BindQuery(&query); err != nil {
		return err
	}
	page, err := api.users.List(c.Request().Context(), query.Page, query.PageSize)
	if err != nil {
		return userTransportError(err)
	}
	items := make([]dto.UserResponse, len(page.Users))
	for index, user := range page.Users {
		items[index] = toUserResponse(user)
	}
	return c.JSON(http.StatusOK, dto.UserListResponse{
		Items: items,
		Pagination: dto.PaginationResponse{
			Page:       page.Page,
			PageSize:   page.PageSize,
			Total:      page.Total,
			TotalPages: page.TotalPages,
		},
	})
}

func (api usersAPI) updateAccess(c *vial.Context) error {
	id, err := userID(c)
	if err != nil {
		return err
	}
	var request dto.UpdateUserAccessRequest
	if err := c.BindJSON(&request); err != nil {
		return err
	}
	user, err := api.users.UpdateAccess(c.Request().Context(), id, request.Role, request.Active)
	if err != nil {
		return userTransportError(err)
	}
	return c.JSON(http.StatusOK, toUserResponse(user))
}

func userID(c *vial.Context) (string, error) {
	id := c.Param("id")
	if !domain.ValidUserID(id) {
		return "", vial.NewHTTPError(http.StatusBadRequest, "invalidUserId", "User ID must be a UUID")
	}
	return id, nil
}

func userTransportError(err error) error {
	var validation *domain.ValidationError
	switch {
	case errors.As(err, &validation):
		return vial.NewHTTPError(http.StatusBadRequest, validation.Code, validation.Message)
	case errors.Is(err, application.ErrUsernameTaken):
		return vial.NewHTTPError(http.StatusConflict, "usernameTaken", "Username is already taken")
	case errors.Is(err, application.ErrUserNotFound):
		return vial.NewHTTPError(http.StatusNotFound, "userNotFound", "User was not found")
	case errors.Is(err, application.ErrUserManagesTeams):
		return vial.NewHTTPError(http.StatusConflict, "userManagesTeams", "Reassign this user's teams before changing their manager access")
	case errors.Is(err, application.ErrInvalidPassword):
		return vial.NewHTTPError(http.StatusBadRequest, "invalidCurrentPassword", "Current password is incorrect")
	default:
		return err
	}
}

func toUserResponse(user domain.User) dto.UserResponse {
	return dto.UserResponse{
		ID:        user.ID,
		Username:  user.Username,
		Role:      user.Role,
		TeamID:    user.TeamID,
		Active:    user.Active,
		CreatedAt: user.CreatedAt,
		UpdatedAt: user.UpdatedAt,
	}
}
