-- +goose Up
-- +goose StatementBegin
DO $$
DECLARE
    replacement_manager UUID;
BEGIN
    IF EXISTS (
        SELECT 1
        FROM teams
        JOIN users ON users.id = teams.manager_id
        WHERE users.role <> 'manager' OR NOT users.active
    ) THEN
        SELECT id INTO replacement_manager
        FROM users
        WHERE role = 'manager' AND active
        ORDER BY created_at, id
        LIMIT 1;

        IF replacement_manager IS NULL THEN
            RAISE EXCEPTION 'an active manager is required to repair existing teams';
        END IF;

        UPDATE teams
        SET manager_id = replacement_manager, updated_at = CURRENT_TIMESTAMP
        WHERE manager_id IN (SELECT id FROM users WHERE role <> 'manager' OR NOT active);
    END IF;
END $$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION require_active_team_manager() RETURNS trigger AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM users
        WHERE id = NEW.manager_id AND role = 'manager' AND active
    ) THEN
        RAISE EXCEPTION 'team manager must be an active manager'
            USING ERRCODE = '23514', CONSTRAINT = 'teams_manager_role_check';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER teams_require_active_manager
BEFORE INSERT OR UPDATE OF manager_id ON teams
FOR EACH ROW EXECUTE FUNCTION require_active_team_manager();

-- +goose StatementBegin
CREATE FUNCTION preserve_managed_team_manager() RETURNS trigger AS $$
BEGIN
    IF (NEW.role <> 'manager' OR NOT NEW.active)
        AND EXISTS (SELECT 1 FROM teams WHERE manager_id = NEW.id) THEN
        RAISE EXCEPTION 'user manages one or more teams'
            USING ERRCODE = '23514', CONSTRAINT = 'users_managed_teams_check';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER users_preserve_managed_team_manager
BEFORE UPDATE OF role, active ON users
FOR EACH ROW EXECUTE FUNCTION preserve_managed_team_manager();

-- +goose Down
DROP TRIGGER users_preserve_managed_team_manager ON users;
DROP FUNCTION preserve_managed_team_manager();
DROP TRIGGER teams_require_active_manager ON teams;
DROP FUNCTION require_active_team_manager();
