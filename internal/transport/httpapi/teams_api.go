package httpapi

import (
	"errors"
	"net/http"

	vial "github.com/jrgf/go-vial"
	"github.com/jrgf/vialboard/internal/application"
	"github.com/jrgf/vialboard/internal/domain"
	"github.com/jrgf/vialboard/internal/transport/httpapi/dto"
)

type teamsAPI struct {
	teams *application.TeamService
	users *application.UserService
}

func (api teamsAPI) register(app *vial.App, authenticated vial.Middleware) {
	app.Get("/teams", authenticated(api.list))
	app.Post("/teams", authenticated(api.create))
	app.Patch("/teams/{id}", authenticated(api.update))
	app.Get("/teams/availableManagers", authenticated(api.availableManagers))
	app.Get("/teams/available-users", authenticated(api.availableUsers))
	app.Get("/teams/{id}/members", authenticated(api.members))
	app.Post("/teams/{id}/users", authenticated(api.createUser))
	app.Put("/teams/{id}/members/{userId}", authenticated(api.addMember))
	app.Delete("/teams/{id}/members/{userId}", authenticated(api.removeMember))
}

func (api teamsAPI) update(c *vial.Context) error {
	actor, err := currentUser(c)
	if err != nil {
		return err
	}
	id, err := teamID(c)
	if err != nil {
		return err
	}
	var request dto.UpdateTeamRequest
	if err := c.BindJSON(&request); err != nil {
		return err
	}
	team, err := api.teams.AssignManager(c.Request().Context(), actor, id, request.ManagerID)
	if err != nil {
		return teamTransportError(err)
	}
	return c.JSON(http.StatusOK, toTeamResponse(team))
}

func (api teamsAPI) list(c *vial.Context) error {
	actor, err := currentUser(c)
	if err != nil {
		return err
	}
	teams, err := api.teams.List(c.Request().Context(), actor)
	if err != nil {
		return teamTransportError(err)
	}
	response := make([]dto.TeamResponse, len(teams))
	for index, team := range teams {
		response[index] = toTeamResponse(team)
	}
	return c.JSON(http.StatusOK, response)
}

func (api teamsAPI) create(c *vial.Context) error {
	actor, err := currentUser(c)
	if err != nil {
		return err
	}
	var request dto.CreateTeamRequest
	if err := c.BindJSON(&request); err != nil {
		return err
	}
	team, err := api.teams.Create(c.Request().Context(), actor, request.Name, request.ManagerID)
	if err != nil {
		return teamTransportError(err)
	}
	return c.JSON(http.StatusCreated, toTeamResponse(team))
}

func (api teamsAPI) availableManagers(c *vial.Context) error {
	actor, err := currentUser(c)
	if err != nil {
		return err
	}
	users, err := api.teams.AvailableManagers(c.Request().Context(), actor)
	if err != nil {
		return teamTransportError(err)
	}
	return c.JSON(http.StatusOK, toMemberResponses(users))
}

func (api teamsAPI) members(c *vial.Context) error {
	actor, err := currentUser(c)
	if err != nil {
		return err
	}
	teamID, err := teamID(c)
	if err != nil {
		return err
	}
	users, err := api.teams.Members(c.Request().Context(), actor, teamID)
	if err != nil {
		return teamTransportError(err)
	}
	return c.JSON(http.StatusOK, toMemberResponses(users))
}

func (api teamsAPI) availableUsers(c *vial.Context) error {
	actor, err := currentUser(c)
	if err != nil {
		return err
	}
	users, err := api.teams.AvailableUsers(c.Request().Context(), actor)
	if err != nil {
		return teamTransportError(err)
	}
	return c.JSON(http.StatusOK, toMemberResponses(users))
}

func (api teamsAPI) createUser(c *vial.Context) error {
	actor, err := currentUser(c)
	if err != nil {
		return err
	}
	teamID, err := teamID(c)
	if err != nil {
		return err
	}
	if _, err := api.teams.RequireManaged(c.Request().Context(), actor, teamID); err != nil {
		return teamTransportError(err)
	}
	var request dto.CreateTeamUserRequest
	if err := c.BindJSON(&request); err != nil {
		return err
	}
	user, err := api.users.CreateInTeam(c.Request().Context(), request.Username, request.Password, teamID)
	if err != nil {
		return userTransportError(err)
	}
	return c.JSON(http.StatusCreated, toUserResponse(user))
}

func (api teamsAPI) addMember(c *vial.Context) error {
	actor, err := currentUser(c)
	if err != nil {
		return err
	}
	teamID, err := teamID(c)
	if err != nil {
		return err
	}
	memberID, err := memberUserID(c)
	if err != nil {
		return err
	}
	user, err := api.teams.AddMember(c.Request().Context(), actor, teamID, memberID)
	if err != nil {
		return teamTransportError(err)
	}
	return c.JSON(http.StatusOK, toUserResponse(user))
}

func (api teamsAPI) removeMember(c *vial.Context) error {
	actor, err := currentUser(c)
	if err != nil {
		return err
	}
	teamID, err := teamID(c)
	if err != nil {
		return err
	}
	memberID, err := memberUserID(c)
	if err != nil {
		return err
	}
	if err := api.teams.RemoveMember(c.Request().Context(), actor, teamID, memberID); err != nil {
		return teamTransportError(err)
	}
	return c.NoContent(http.StatusNoContent)
}

func teamID(c *vial.Context) (string, error) {
	id := c.Param("id")
	if !domain.ValidTeamID(id) {
		return "", vial.NewHTTPError(http.StatusBadRequest, "invalidTeamId", "Team ID must be a UUID")
	}
	return id, nil
}

func memberUserID(c *vial.Context) (string, error) {
	id := c.Param("userId")
	if !domain.ValidUserID(id) {
		return "", vial.NewHTTPError(http.StatusBadRequest, "invalidUserId", "User ID must be a UUID")
	}
	return id, nil
}

func teamTransportError(err error) error {
	var validation *domain.ValidationError
	switch {
	case errors.As(err, &validation):
		return vial.NewHTTPError(http.StatusBadRequest, validation.Code, validation.Message)
	case errors.Is(err, domain.ErrTeamNotFound):
		return vial.NewHTTPError(http.StatusNotFound, "teamNotFound", "Team was not found")
	case errors.Is(err, application.ErrUserNotFound):
		return vial.NewHTTPError(http.StatusNotFound, "userNotFound", "User was not found")
	case errors.Is(err, application.ErrTeamNameTaken):
		return vial.NewHTTPError(http.StatusConflict, "teamNameTaken", "Team name is already taken")
	case errors.Is(err, application.ErrUserHasTeam):
		return vial.NewHTTPError(http.StatusConflict, "userHasTeam", "User already belongs to another team")
	case errors.Is(err, application.ErrTeamMemberNotFound):
		return vial.NewHTTPError(http.StatusNotFound, "teamMemberNotFound", "User does not belong to this team")
	case errors.Is(err, application.ErrTeamForbidden):
		return vial.NewHTTPError(http.StatusForbidden, "teamForbidden", "You cannot manage this team")
	default:
		return err
	}
}

func toTeamResponse(team domain.Team) dto.TeamResponse {
	return dto.TeamResponse{ID: team.ID, Name: team.Name, ManagerID: team.ManagerID, CreatedAt: team.CreatedAt, UpdatedAt: team.UpdatedAt}
}

func toMemberResponses(users []domain.User) []dto.MemberResponse {
	members := make([]dto.MemberResponse, len(users))
	for index, user := range users {
		members[index] = dto.MemberResponse{ID: user.ID, Username: user.Username, Role: user.Role, TeamID: user.TeamID}
	}
	return members
}
