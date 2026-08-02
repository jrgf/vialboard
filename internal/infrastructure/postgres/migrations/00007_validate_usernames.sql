-- +goose Up
ALTER TABLE users
    ADD CONSTRAINT users_username_format_check
    CHECK (username ~ '^[A-Za-z0-9][A-Za-z0-9_-]{0,15}$');

-- +goose Down
ALTER TABLE users DROP CONSTRAINT users_username_format_check;
