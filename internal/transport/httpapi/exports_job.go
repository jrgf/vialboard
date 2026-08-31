package httpapi

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"net/url"
	"strconv"
	"time"

	vial "github.com/jrgf/go-vial"
	"github.com/jrgf/vialboard/internal/application"
	"github.com/jrgf/vialboard/internal/domain"
)

type issueExportResult struct {
	DownloadURL string `json:"download_url"`
	Filename    string `json:"filename"`
	Rows        int    `json:"rows"`
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
	if err := api.store.save(ctx, job.ID(), ownerID, filename, buffer.Bytes()); err != nil {
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
