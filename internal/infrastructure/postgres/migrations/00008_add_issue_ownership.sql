-- +goose Up
ALTER TABLE issues ADD COLUMN created_by UUID;

UPDATE issues
SET created_by = (
    SELECT id
    FROM users
    WHERE role = 'admin'
    ORDER BY created_at, id
    LIMIT 1
)
WHERE created_by IS NULL;

ALTER TABLE issues
    ALTER COLUMN created_by SET NOT NULL,
    ADD CONSTRAINT issues_created_by_fkey
        FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE RESTRICT;

CREATE INDEX issues_created_by_id_idx ON issues (created_by, id);

-- +goose Down
DROP INDEX issues_created_by_id_idx;
ALTER TABLE issues DROP CONSTRAINT issues_created_by_fkey;
ALTER TABLE issues DROP COLUMN created_by;
