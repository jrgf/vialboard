-- +goose Up
WITH duplicate_sessions AS (
    SELECT
        id,
        row_number() OVER (PARTITION BY user_id ORDER BY created_at DESC, id DESC) AS position
    FROM sessions
    WHERE revoked_at IS NULL
)
UPDATE sessions
SET revoked_at = GREATEST(sessions.created_at, CURRENT_TIMESTAMP)
FROM duplicate_sessions
WHERE sessions.id = duplicate_sessions.id
  AND duplicate_sessions.position > 1;

CREATE UNIQUE INDEX sessions_one_current_per_user_key
    ON sessions (user_id)
    WHERE revoked_at IS NULL;

-- +goose Down
DROP INDEX sessions_one_current_per_user_key;
