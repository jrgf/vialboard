package httpapi

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
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
	"github.com/jrgf/vialboard/internal/application"
	"github.com/jrgf/vialboard/internal/domain"
)

const issueExportOperation = "issues.export.csv"

type issueExportAPI struct {
	issues   *application.IssueService
	users    *application.UserService
	database *sql.DB
	executor *asyncpostgres.Executor
}

type issueExportResult struct {
	DownloadURL string `json:"download_url"`
	Filename    string `json:"filename"`
	Rows        int    `json:"rows"`
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

func (api issueExportAPI) generate(ctx context.Context, job vial.AsyncJob) (any, error) {
	ownerID := job.Metadata()["user_id"]
	actor, err := api.users.GetByID(ctx, ownerID)
	if errors.Is(err, application.ErrUserNotFound) || (err == nil && !actor.Active) {
		return nil, &vial.OperationError{Code: "export_owner_unavailable", Message: "The export owner is no longer active"}
	}
	if err != nil {
		return nil, err
	}
	if err := job.Progress(ctx, 5); err != nil {
		return nil, err
	}

	// ponytail: one export is buffered in memory; move payloads to object storage when exports outgrow worker memory.
	var buffer bytes.Buffer
	writer := csv.NewWriter(&buffer)
	if err := writer.Write([]string{"id", "title", "description", "status", "priority", "due_date", "created_by", "team_id", "assignee_id", "created_at", "updated_at"}); err != nil {
		return nil, err
	}

	rows := 0
	for page := 1; ; page++ {
		result, err := api.issues.List(ctx, actor, "", "", "", "", "createdAt", "asc", page, 100)
		if err != nil {
			return nil, err
		}
		for _, issue := range result.Issues {
			if err := writeIssueCSV(writer, issue); err != nil {
				return nil, err
			}
			rows++
		}
		writer.Flush()
		if err := writer.Error(); err != nil {
			return nil, err
		}
		if err := job.Progress(ctx, min(90, 5+page*85/result.TotalPages)); err != nil {
			return nil, err
		}
		if page >= result.TotalPages {
			break
		}
	}

	filename := "vialboard-issues-" + time.Now().UTC().Format("2006-01-02") + ".csv"
	_, err = api.database.ExecContext(ctx, `
INSERT INTO issue_exports (operation_id, owner_id, filename, content)
VALUES ($1, $2, $3, $4)
ON CONFLICT (operation_id) DO UPDATE
SET owner_id = EXCLUDED.owner_id, filename = EXCLUDED.filename, content = EXCLUDED.content, created_at = CURRENT_TIMESTAMP`,
		job.ID(), ownerID, filename, buffer.Bytes())
	if err != nil {
		return nil, err
	}
	if err := job.Progress(ctx, 100); err != nil {
		return nil, err
	}
	return issueExportResult{
		DownloadURL: "/exports/issues/" + url.PathEscape(job.ID()) + "/download",
		Filename:    filename,
		Rows:        rows,
	}, nil
}

func writeIssueCSV(writer *csv.Writer, issue domain.Issue) error {
	return writer.Write([]string{
		strconv.FormatUint(issue.ID, 10), issue.Title, issue.Description,
		string(issue.Status), string(issue.Priority), dateValue(issue.DueDate),
		issue.CreatedBy, stringValue(issue.TeamID), stringValue(issue.AssigneeID),
		issue.CreatedAt.UTC().Format(time.RFC3339), issue.UpdatedAt.UTC().Format(time.RFC3339),
	})
}

func dateValue(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.Format("2006-01-02")
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
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
	var filename string
	var content []byte
	if err := api.database.QueryRowContext(c.Request().Context(), `
SELECT filename, content FROM issue_exports WHERE operation_id = $1 AND owner_id = $2`,
		operation.ID, actor.ID).Scan(&filename, &content); errors.Is(err, sql.ErrNoRows) {
		return vial.NewHTTPError(http.StatusNotFound, "exportNotFound", "The export was not found")
	} else if err != nil {
		return err
	}

	response := c.Response()
	response.Header().Set("Content-Type", "text/csv; charset=utf-8")
	response.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": filename}))
	response.Header().Set("Content-Length", strconv.Itoa(len(content)))
	response.WriteHeader(http.StatusOK)
	_, err = response.Write(content)
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
	if err := disableWriteDeadline(response); err != nil {
		return fmt.Errorf("disable operation stream write deadline: %w", err)
	}
	flusher, ok := response.(http.Flusher)
	if !ok {
		return errors.New("operation stream does not support flushing")
	}
	response.Header().Set("Content-Type", "text/event-stream")
	response.Header().Set("Cache-Control", "no-cache, no-store")
	response.Header().Set("X-Accel-Buffering", "no")
	if _, err := fmt.Fprint(response, "retry: 2000\n\n"); err != nil {
		return nil
	}
	flusher.Flush()

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

	heartbeat := time.NewTicker(15 * time.Second)
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
			payload, err := json.Marshal(result.operation)
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintf(response, "event: completion\ndata: %s\n\n", payload); err != nil {
				return nil
			}
			flusher.Flush()
			return nil
		case <-heartbeat.C:
			current, err := api.users.Authenticate(c.Request().Context(), token)
			if err != nil || current.ID != actor.ID {
				return nil
			}
			if _, err := fmt.Fprint(response, ": heartbeat\n\n"); err != nil {
				return nil
			}
			flusher.Flush()
		}
	}
}
