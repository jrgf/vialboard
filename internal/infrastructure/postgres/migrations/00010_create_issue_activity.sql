-- +goose Up
CREATE TABLE issue_activities (
    id BIGSERIAL PRIMARY KEY,
    issue_id BIGINT NOT NULL REFERENCES issues(id) ON DELETE CASCADE,
    actor_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    kind TEXT NOT NULL CHECK (kind IN (
        'created',
        'updated',
        'statusChanged',
        'priorityChanged',
        'dueDateChanged',
        'assignmentChanged',
        'comment'
    )),
    body TEXT NOT NULL DEFAULT '' CHECK (kind <> 'comment' OR (btrim(body) <> '' AND char_length(body) <= 2000)),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX issue_activities_issue_id_idx ON issue_activities (issue_id, id DESC);

-- +goose Down
DROP TABLE issue_activities;
