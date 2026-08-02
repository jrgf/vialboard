-- +goose Up
CREATE TABLE notifications (
    id BIGSERIAL PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind TEXT NOT NULL CHECK (kind IN ('issueCreated', 'issueUpdated', 'issueCommented', 'teamAdded', 'teamRemoved')),
    message TEXT NOT NULL CHECK (char_length(btrim(message)) BETWEEN 1 AND 300),
    issue_id BIGINT REFERENCES issues(id) ON DELETE SET NULL,
    team_id UUID REFERENCES teams(id) ON DELETE SET NULL,
    read_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX notifications_user_id_idx ON notifications (user_id, id DESC);
CREATE INDEX notifications_unread_idx ON notifications (user_id, id DESC) WHERE read_at IS NULL;

-- +goose Down
DROP TABLE notifications;
