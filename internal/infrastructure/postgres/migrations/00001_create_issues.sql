-- +goose Up
CREATE TABLE IF NOT EXISTS issues (
    id BIGSERIAL PRIMARY KEY,
    title TEXT NOT NULL,
    description TEXT NOT NULL,
    status TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

UPDATE issues SET created_at = CURRENT_TIMESTAMP WHERE created_at IS NULL;
UPDATE issues SET updated_at = CURRENT_TIMESTAMP WHERE updated_at IS NULL;

ALTER TABLE issues
    ALTER COLUMN created_at SET DEFAULT CURRENT_TIMESTAMP,
    ALTER COLUMN created_at SET NOT NULL,
    ALTER COLUMN updated_at SET DEFAULT CURRENT_TIMESTAMP,
    ALTER COLUMN updated_at SET NOT NULL;

-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'issues_status_check'
          AND conrelid = 'issues'::regclass
    ) THEN
        ALTER TABLE issues
            ADD CONSTRAINT issues_status_check CHECK (status IN ('open', 'closed'));
    END IF;
END
$$;
-- +goose StatementEnd

-- +goose Down
DROP TABLE IF EXISTS issues;
