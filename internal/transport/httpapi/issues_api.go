package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	vial "github.com/jrgf/go-vial"
	"github.com/jrgf/vialboard/internal/application"
	"github.com/jrgf/vialboard/internal/domain"
	"github.com/jrgf/vialboard/internal/transport/httpapi/dto"
)

type issuesAPI struct {
	issues *application.IssueService
}

func (api issuesAPI) register(app *vial.App, authenticated vial.Middleware) {
	app.Get("/issues", authenticated(api.list))
	app.Post("/issues", authenticated(api.create))
	app.Get("/issues/{id}", authenticated(api.get))
	app.Patch("/issues/{id}", authenticated(api.update))
	app.Delete("/issues/{id}", authenticated(api.delete))
	app.Get("/issues/{id}/activity", authenticated(api.listActivity))
	app.Post("/issues/{id}/comments", authenticated(api.addComment))
}

func (api issuesAPI) list(c *vial.Context) error {
	actor, err := currentUser(c)
	if err != nil {
		return err
	}
	var query dto.ListIssuesQuery
	if err := c.BindQuery(&query); err != nil {
		return err
	}
	page, err := api.issues.List(c.Request().Context(), actor, query.Status, query.Priority, query.TeamID, query.Search, query.Sort, query.Order, query.Page, query.PageSize)
	if err != nil {
		return issueTransportError(err)
	}
	items := make([]dto.IssueResponse, len(page.Issues))
	for index, issue := range page.Issues {
		items[index] = toIssueResponse(issue)
	}
	return c.JSON(http.StatusOK, dto.IssueListResponse{
		Items: items,
		Pagination: dto.PaginationResponse{
			Page:       page.Page,
			PageSize:   page.PageSize,
			Total:      page.Total,
			TotalPages: page.TotalPages,
		},
	})
}

func (api issuesAPI) create(c *vial.Context) error {
	actor, err := currentUser(c)
	if err != nil {
		return err
	}
	var request dto.CreateIssueRequest
	if err := c.BindJSON(&request); err != nil {
		return err
	}
	issue, err := api.issues.Create(c.Request().Context(), actor, request.Title, request.Description, request.Status, request.Priority, request.DueDate, request.TeamID, request.AssigneeID)
	if err != nil {
		return issueTransportError(err)
	}
	return c.JSON(http.StatusCreated, toIssueResponse(issue))
}

func (api issuesAPI) get(c *vial.Context) error {
	actor, err := currentUser(c)
	if err != nil {
		return err
	}
	id, err := issueID(c)
	if err != nil {
		return err
	}
	issue, err := api.issues.Get(c.Request().Context(), actor, id)
	if err != nil {
		return issueTransportError(err)
	}
	return c.JSON(http.StatusOK, toIssueResponse(issue))
}

func (api issuesAPI) update(c *vial.Context) error {
	actor, err := currentUser(c)
	if err != nil {
		return err
	}
	id, err := issueID(c)
	if err != nil {
		return err
	}
	var request dto.UpdateIssueRequest
	if err := c.BindJSON(&request); err != nil {
		return err
	}
	issue, err := api.issues.Update(c.Request().Context(), actor, id, request.Title, request.Description, request.Status, request.Priority, request.DueDate, request.TeamID, request.AssigneeID)
	if err != nil {
		return issueTransportError(err)
	}
	return c.JSON(http.StatusOK, toIssueResponse(issue))
}

func (api issuesAPI) delete(c *vial.Context) error {
	actor, err := currentUser(c)
	if err != nil {
		return err
	}
	id, err := issueID(c)
	if err != nil {
		return err
	}
	if err := api.issues.Delete(c.Request().Context(), actor, id); err != nil {
		return issueTransportError(err)
	}
	return c.NoContent(http.StatusNoContent)
}

func (api issuesAPI) listActivity(c *vial.Context) error {
	actor, err := currentUser(c)
	if err != nil {
		return err
	}
	id, err := issueID(c)
	if err != nil {
		return err
	}
	var query dto.ListActivityQuery
	if err := c.BindQuery(&query); err != nil {
		return err
	}
	page, err := api.issues.ListActivity(c.Request().Context(), actor, id, query.Page, query.PageSize)
	if err != nil {
		return issueTransportError(err)
	}
	items := make([]dto.ActivityResponse, len(page.Items))
	for index, activity := range page.Items {
		items[index] = toActivityResponse(activity)
	}
	return c.JSON(http.StatusOK, dto.ActivityListResponse{
		Items: items,
		Pagination: dto.PaginationResponse{
			Page: page.Page, PageSize: page.PageSize, Total: page.Total, TotalPages: page.TotalPages,
		},
	})
}

func (api issuesAPI) addComment(c *vial.Context) error {
	actor, err := currentUser(c)
	if err != nil {
		return err
	}
	id, err := issueID(c)
	if err != nil {
		return err
	}
	var request dto.CreateCommentRequest
	if err := c.BindJSON(&request); err != nil {
		return err
	}
	activity, err := api.issues.AddComment(c.Request().Context(), actor, id, request.Body)
	if err != nil {
		return issueTransportError(err)
	}
	return c.JSON(http.StatusCreated, toActivityResponse(activity))
}

func issueID(c *vial.Context) (uint64, error) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		return 0, vial.NewHTTPError(http.StatusBadRequest, "invalidIssueId", "Issue ID must be a positive integer")
	}
	return id, nil
}

func issueTransportError(err error) error {
	var validation *domain.ValidationError
	switch {
	case errors.As(err, &validation):
		return vial.NewHTTPError(http.StatusBadRequest, validation.Code, validation.Message)
	case errors.Is(err, domain.ErrIssueNotFound):
		return vial.NewHTTPError(http.StatusNotFound, "issueNotFound", "Issue was not found")
	case errors.Is(err, domain.ErrIssueForbidden):
		return vial.NewHTTPError(http.StatusForbidden, "issueForbidden", "You cannot perform this action")
	default:
		return err
	}
}

func toIssueResponse(issue domain.Issue) dto.IssueResponse {
	var dueDate *string
	if issue.DueDate != nil {
		value := issue.DueDate.Format(time.DateOnly)
		dueDate = &value
	}
	return dto.IssueResponse{
		ID:          issue.ID,
		Title:       issue.Title,
		Description: issue.Description,
		Status:      issue.Status,
		Priority:    issue.Priority,
		DueDate:     dueDate,
		CreatedBy:   issue.CreatedBy,
		TeamID:      issue.TeamID,
		AssigneeID:  issue.AssigneeID,
		CreatedAt:   issue.CreatedAt,
		UpdatedAt:   issue.UpdatedAt,
	}
}

func toActivityResponse(activity domain.IssueActivity) dto.ActivityResponse {
	return dto.ActivityResponse{
		ID:            activity.ID,
		IssueID:       activity.IssueID,
		ActorID:       activity.ActorID,
		ActorUsername: activity.ActorUsername,
		Kind:          activity.Kind,
		Body:          activity.Body,
		CreatedAt:     activity.CreatedAt,
	}
}
