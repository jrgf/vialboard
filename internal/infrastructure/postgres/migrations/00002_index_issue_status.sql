-- +goose Up
CREATE INDEX IF NOT EXISTS idx_issues_status ON issues (status);

-- +goose Down
DROP INDEX IF EXISTS idx_issues_status;
