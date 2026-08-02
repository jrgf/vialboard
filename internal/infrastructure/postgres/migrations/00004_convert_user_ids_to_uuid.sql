-- +goose Up
ALTER TABLE users
    ADD COLUMN id_uuid UUID NOT NULL DEFAULT gen_random_uuid();

ALTER TABLE sessions
    ADD COLUMN user_id_uuid UUID;

UPDATE sessions
SET user_id_uuid = users.id_uuid
FROM users
WHERE sessions.user_id = users.id;

ALTER TABLE sessions
    ALTER COLUMN user_id_uuid SET NOT NULL,
    DROP CONSTRAINT sessions_user_id_fkey;

ALTER TABLE users DROP CONSTRAINT users_pkey;
ALTER TABLE sessions DROP COLUMN user_id;
ALTER TABLE users DROP COLUMN id;
ALTER TABLE users RENAME COLUMN id_uuid TO id;
ALTER TABLE sessions RENAME COLUMN user_id_uuid TO user_id;
ALTER TABLE users ADD PRIMARY KEY (id);
ALTER TABLE sessions
    ADD CONSTRAINT sessions_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
CREATE INDEX sessions_user_id_idx ON sessions (user_id);

-- +goose Down
ALTER TABLE users
    ADD COLUMN id_bigint BIGSERIAL;

ALTER TABLE sessions
    ADD COLUMN user_id_bigint BIGINT;

UPDATE sessions
SET user_id_bigint = users.id_bigint
FROM users
WHERE sessions.user_id = users.id;

ALTER TABLE sessions
    ALTER COLUMN user_id_bigint SET NOT NULL,
    DROP CONSTRAINT sessions_user_id_fkey;

ALTER TABLE users DROP CONSTRAINT users_pkey;
ALTER TABLE sessions DROP COLUMN user_id;
ALTER TABLE users DROP COLUMN id;
ALTER TABLE users RENAME COLUMN id_bigint TO id;
ALTER TABLE sessions RENAME COLUMN user_id_bigint TO user_id;
ALTER TABLE users ADD PRIMARY KEY (id);
ALTER TABLE sessions
    ADD CONSTRAINT sessions_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
CREATE INDEX sessions_user_id_idx ON sessions (user_id);
