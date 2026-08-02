-- +goose Up
ALTER TABLE users
    ADD COLUMN role TEXT NOT NULL DEFAULT 'viewer'
    CONSTRAINT users_role_check CHECK (role IN ('viewer', 'admin'));

UPDATE users SET role = 'admin';

-- +goose Down
ALTER TABLE users DROP COLUMN role;
