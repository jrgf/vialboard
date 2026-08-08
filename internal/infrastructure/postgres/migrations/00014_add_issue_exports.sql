-- +goose Up
CREATE TABLE issue_exports (
    operation_id TEXT PRIMARY KEY,
    owner_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    filename TEXT NOT NULL,
    content BYTEA NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX issue_exports_owner_created_idx
    ON issue_exports (owner_id, created_at DESC);

-- +goose Down
DROP TABLE issue_exports;
