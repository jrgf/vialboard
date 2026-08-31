package httpapi

import (
	"context"
	"database/sql"
	"errors"
)

var errIssueExportNotFound = errors.New("issue export not found")

type issueExportStore struct {
	database *sql.DB
}

type issueExportFile struct {
	filename string
	content  []byte
}

func (store issueExportStore) save(ctx context.Context, operationID, ownerID, filename string, content []byte) error {
	_, err := store.database.ExecContext(ctx, `
INSERT INTO issue_exports (operation_id, owner_id, filename, content)
VALUES ($1, $2, $3, $4)
ON CONFLICT (operation_id) DO UPDATE
SET owner_id = EXCLUDED.owner_id, filename = EXCLUDED.filename, content = EXCLUDED.content, created_at = CURRENT_TIMESTAMP`,
		operationID, ownerID, filename, content)
	return err
}

func (store issueExportStore) get(ctx context.Context, operationID, ownerID string) (issueExportFile, error) {
	var file issueExportFile
	err := store.database.QueryRowContext(ctx, `
SELECT filename, content FROM issue_exports WHERE operation_id = $1 AND owner_id = $2`,
		operationID, ownerID).Scan(&file.filename, &file.content)
	if errors.Is(err, sql.ErrNoRows) {
		return issueExportFile{}, errIssueExportNotFound
	}
	return file, err
}
