package httpapi

import (
	"context"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	vial "github.com/jrgf/go-vial"
	"github.com/jrgf/go-vial/contrib/asyncpostgres"
	"github.com/jrgf/go-vial/sse"
	"github.com/jrgf/vialboard/internal/application"
)

const issueExportOperation = "issues.export.csv"

type issueExportAPI struct {
	issues   *application.IssueService
	users    *application.UserService
	store    issueExportStore
	executor *asyncpostgres.Executor
}

func (api issueExportAPI) register(group *vial.Group) {
	group.Post("/exports/issues", api.submit)
	group.Get("/exports/issues/{id}/download", api.download)
	group.Get("/operations/{id}", vial.OperationStatusHandler(api.executor, api.authorize))
	group.Delete("/operations/{id}", vial.OperationCancelHandler(api.executor, api.authorize))
	group.Get("/operations/{id}/events", api.events)
}

func (api issueExportAPI) submit(c *vial.Context) error {
	actor, err := currentUser(c)
	if err != nil {
		return err
	}
	operation, err := api.executor.Submit(c.Request().Context(), vial.SubmitRequest{
		Name:             issueExportOperation,
		IdempotencyKey:   c.Header("Idempotency-Key"),
		IdempotencyScope: actor.ID,
		Metadata:         map[string]string{"user_id": actor.ID},
		Retry: vial.RetryPolicy{
			MaxAttempts:    3,
			InitialBackoff: 250 * time.Millisecond,
			MaxBackoff:     2 * time.Second,
		},
	})
	if err != nil {
		return err
	}
	return c.AcceptedAt(operation, "/operations/"+url.PathEscape(operation.ID))
}

func (api issueExportAPI) authorize(c *vial.Context, operation *vial.Operation) error {
	actor, err := currentUser(c)
	if err != nil {
		return err
	}
	if operation.Metadata["user_id"] != actor.ID {
		return vial.NewHTTPError(http.StatusNotFound, "operationNotFound", "The operation was not found")
	}
	return nil
}

func (api issueExportAPI) download(c *vial.Context) error {
	operation, err := api.executor.Get(c.Request().Context(), c.Param("id"))
	if err != nil {
		return err
	}
	if err := api.authorize(c, operation); err != nil {
		return err
	}
	if operation.Status != vial.OperationSucceeded {
		return vial.NewHTTPError(http.StatusConflict, "exportNotReady", "The export is not ready")
	}

	actor, _ := currentUser(c)
	file, err := api.store.get(c.Request().Context(), operation.ID, actor.ID)
	if errors.Is(err, errIssueExportNotFound) {
		return vial.NewHTTPError(http.StatusNotFound, "exportNotFound", "The export was not found")
	} else if err != nil {
		return err
	}

	response := c.Response()
	response.Header().Set("Content-Type", "text/csv; charset=utf-8")
	response.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": file.filename}))
	response.Header().Set("Content-Length", strconv.Itoa(len(file.content)))
	response.WriteHeader(http.StatusOK)
	_, err = response.Write(file.content)
	return err
}

func (api issueExportAPI) events(c *vial.Context) error {
	operation, err := api.executor.Get(c.Request().Context(), c.Param("id"))
	if err != nil {
		return err
	}
	if err := api.authorize(c, operation); err != nil {
		return err
	}
	actor, _ := currentUser(c)
	parts := strings.Fields(c.Header("Authorization"))
	token := parts[1]

	response := c.Response()
	response.Header().Set("Content-Type", "text/event-stream")
	response.Header().Set("Cache-Control", "no-cache, no-store")
	response.Header().Set("X-Accel-Buffering", "no")
	if !writeEventStream(response, func() error {
		_, err := fmt.Fprint(response, "retry: 2000\n\n")
		return err
	}) {
		return nil
	}

	completed := make(chan struct {
		operation *vial.Operation
		err       error
	}, 1)
	go func() {
		current, waitErr := api.executor.Wait(c.Request().Context(), operation.ID)
		completed <- struct {
			operation *vial.Operation
			err       error
		}{current, waitErr}
	}()

	heartbeat := time.NewTicker(sse.DefaultHeartbeat)
	defer heartbeat.Stop()
	for {
		select {
		case <-c.Request().Context().Done():
			return nil
		case result := <-completed:
			if result.err != nil {
				if errors.Is(result.err, context.Canceled) {
					return nil
				}
				return result.err
			}
			event, err := sse.JSON("completion", result.operation)
			if err != nil {
				return err
			}
			if !writeEventStream(response, func() error { return sse.WriteEvent(response, event) }) {
				return nil
			}
			return nil
		case <-heartbeat.C:
			current, err := api.users.Authenticate(c.Request().Context(), token)
			if err != nil || current.ID != actor.ID {
				return nil
			}
			if !writeEventStream(response, func() error { return sse.WriteComment(response, "heartbeat") }) {
				return nil
			}
		}
	}
}
