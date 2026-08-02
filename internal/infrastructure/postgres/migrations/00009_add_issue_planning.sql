-- +goose Up
ALTER TABLE issues
    ADD COLUMN priority TEXT NOT NULL DEFAULT 'medium'
        CONSTRAINT issues_priority_check CHECK (priority IN ('low', 'medium', 'high', 'critical')),
    ADD COLUMN due_date DATE,
    ADD COLUMN assignee_id UUID REFERENCES users(id) ON DELETE SET NULL;

CREATE INDEX issues_assignee_id_idx ON issues (assignee_id, id);
CREATE INDEX issues_priority_idx ON issues (priority, id);
CREATE INDEX issues_due_date_idx ON issues (due_date, id) WHERE due_date IS NOT NULL;

-- +goose Down
DROP INDEX issues_due_date_idx;
DROP INDEX issues_priority_idx;
DROP INDEX issues_assignee_id_idx;
ALTER TABLE issues
    DROP COLUMN assignee_id,
    DROP COLUMN due_date,
    DROP COLUMN priority;
