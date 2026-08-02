-- +goose Up
ALTER TABLE users DROP CONSTRAINT users_role_check;
ALTER TABLE users ADD CONSTRAINT users_role_check CHECK (role IN ('viewer', 'manager', 'admin'));

CREATE TABLE teams (
    id UUID PRIMARY KEY,
    name TEXT NOT NULL CHECK (char_length(btrim(name)) BETWEEN 1 AND 64),
    manager_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX teams_name_lower_key ON teams (lower(name));
CREATE INDEX teams_manager_id_idx ON teams (manager_id, id);

ALTER TABLE users ADD COLUMN team_id UUID REFERENCES teams(id) ON DELETE SET NULL;
CREATE INDEX users_team_id_idx ON users (team_id, id);

ALTER TABLE issues ADD COLUMN team_id UUID REFERENCES teams(id) ON DELETE SET NULL;
CREATE INDEX issues_team_id_idx ON issues (team_id, id);

ALTER TABLE issue_activities DROP CONSTRAINT issue_activities_kind_check;
ALTER TABLE issue_activities ADD CONSTRAINT issue_activities_kind_check CHECK (kind IN (
    'created',
    'updated',
    'statusChanged',
    'priorityChanged',
    'dueDateChanged',
    'assignmentChanged',
    'teamChanged',
    'comment'
));

-- +goose Down
UPDATE issue_activities SET kind = 'updated' WHERE kind = 'teamChanged';
ALTER TABLE issue_activities DROP CONSTRAINT issue_activities_kind_check;
ALTER TABLE issue_activities ADD CONSTRAINT issue_activities_kind_check CHECK (kind IN (
    'created',
    'updated',
    'statusChanged',
    'priorityChanged',
    'dueDateChanged',
    'assignmentChanged',
    'comment'
));

DROP INDEX issues_team_id_idx;
ALTER TABLE issues DROP COLUMN team_id;

DROP INDEX users_team_id_idx;
ALTER TABLE users DROP COLUMN team_id;

DROP TABLE teams;

UPDATE users SET role = 'viewer' WHERE role = 'manager';
ALTER TABLE users DROP CONSTRAINT users_role_check;
ALTER TABLE users ADD CONSTRAINT users_role_check CHECK (role IN ('viewer', 'admin'));
