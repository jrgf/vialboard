# Vialboard

Vialboard is a  issue tracker app demonstrating the [go-vial](https://github.com/jrgf/go-vial) framework with GORM and PostgreSQL.

## Architecture

- `cmd/vialboard`: application composition and commands
- `internal/domain`: issue entity and business rules
- `internal/application`: use cases and repository boundary
- `internal/infrastructure/postgres`: GORM repository and migrations
- `internal/transport/httpapi`: go-vial HTTP delivery
- `internal/transport/httpapi/dashboard/static`: ES modules and component-scoped CSS files

## Run

```sh
docker compose up --build
```

The visual dashboard and API listen on `http://localhost:8080`; PostgreSQL listens on `localhost:5432`. The dashboard supports authentication, issue planning, assignment, comments, activity history, and admin user controls.

| Method | Path | Action |
| --- | --- | --- |
| `POST` | `/register` | Register a viewer account and create its bearer session |
| `POST` | `/login` | Create a bearer session |
| `POST` | `/logout` | Revoke the current bearer session |
| `GET` | `/members` | List active members for assignment |
| `PATCH` | `/account/password` | Change the current user's password |
| `GET` | `/teams` | List teams visible to the current user |
| `POST` | `/teams` | Create a manager-owned team (manager/admin; admins send `managerId`) |
| `PATCH` | `/teams/{id}` | Assign an active manager to a team (admin) |
| `GET` | `/teams/availableManagers` | List active managers (admin) |
| `GET` | `/teams/{id}/members` | List the team manager and workers |
| `POST` | `/teams/{id}/users` | Create a worker in a team (manager/admin) |
| `PUT` | `/teams/{id}/members/{userId}` | Add an existing worker (manager/admin) |
| `DELETE` | `/teams/{id}/members/{userId}` | Remove and unassign a worker (manager/admin) |
| `GET` | `/users` | List users (admin) |
| `POST` | `/users` | Create a user (admin) |
| `PATCH` | `/users/{id}` | Change role or activation (admin) |
| `PATCH` | `/users/{id}/password` | Reset a user's password (admin) |
| `GET` | `/issues` | List assigned issues as a worker, managed-team issues as a manager, or all as admin |
| `POST` | `/issues` | Create an issue (manager/admin) |
| `GET` | `/issues/{id}` | Get an issue visible under the same role scope |
| `PATCH` | `/issues/{id}` | Update a visible issue; workers may only change `status` |
| `DELETE` | `/issues/{id}` | Delete a managed-team issue as manager, or any issue as admin |
| `GET` | `/issues/{id}/activity` | List visible issue activity |
| `POST` | `/issues/{id}/comments` | Comment on a visible issue |
| `GET` | `/notifications` | List the current user's notifications |
| `GET` | `/notifications/stream` | Stream realtime notification events |
| `PATCH` | `/notifications/{id}/read` | Mark a notification as read |
| `POST` | `/notifications/readAll` | Mark all notifications as read |
| `GET` | `/health/live` | Process liveness |
| `GET` | `/health/ready` | PostgreSQL readiness |

Issue statuses are `open` and `closed`; priorities are `low`, `medium`, `high`, and `critical`. Create requests require `title`; `description`, `status`, `priority`, `dueDate` (`YYYY-MM-DD`), `teamId`, and `assigneeId` are otherwise optional. Manager-created issues require `teamId`, and assignees must belong to that team.

List issues with optional `status`, `priority`, `search`, `sort`, `order`, `page`, and `pageSize` query parameters. Sort fields are `createdAt`, `updatedAt`, `dueDate`, `priority`, and `title`; order is `asc` or `desc`.

```text
GET /issues?status=open&priority=high&search=release&sort=dueDate&order=asc&page=1&pageSize=20
```

Responses contain `items` and `pagination` metadata. Page defaults to `1`, page size defaults to `20`, and the maximum page size is `100`.

REST JSON fields and query parameters use camelCase.

## Authentication

Issue access requires authentication. Login with `POST /login` using camelCase JSON fields `username` and `password`. The response contains a 24-hour opaque token, `expiresAt`, and the user's role. Send the token as `Authorization: Bearer <token>`. Logging in again immediately revokes the user's previous session.

Register publicly with `POST /register` using `username`, `password`, and matching `passwordConfirmation` fields. The response has the same `token`, `expiresAt`, and `user` fields as login. Registration always creates an active viewer; role and activation can only be changed by an admin. Usernames contain 1–16 ASCII letters, numbers, underscores, or hyphens and must start with a letter or number. Passwords contain 8–250 characters. Registration and login are limited to 10 requests per minute per client address and route.

Logout with `POST /logout` and the same authorization header. The token is rejected immediately after logout. Password changes and admin resets require matching `newPassword` and `passwordConfirmation` fields and revoke existing sessions.

Roles are `viewer` (also accepted as the `worker` alias), `manager`, and `admin`. Workers may belong to one team, see its manager and teammates in a read-only team view, see only directly assigned issues, and change only issue status. Managers create and manage their own teams and workers, and can create, view, update, and delete every issue in those teams. Admins retain global access and assign an active manager when creating a team. Managers with teams cannot be demoted or deactivated. Issue notifications follow the same assignment/team boundary. Removing a worker from a team also clears their assignments for that team's issues. Other cross-team resource requests return `404`. Password changes and resets revoke the user's current session. New passwords use Argon2id while existing bcrypt hashes remain compatible; only a SHA-256 digest of each session token is stored. Use TLS outside local development because bearer tokens grant access until they expire.

Admin user listing supports the same `page` and `pageSize` query parameters as issue listing. Admins can create users with a `viewer`, `manager`, or `admin` role and patch `role` and/or `active`.

Create a user from `VIAL_USER_USERNAME`, `VIAL_USER_PASSWORD`, and optional `VIAL_USER_ROLE` (`viewer` by default):

```sh
docker compose run --rm api user create
```

Compose configures the local `vialboard` / `vialboard` user as an admin.

`BindJSON` rejects malformed bodies and unknown fields, while `BindQuery` converts tagged query values. Domain and application layers enforce business validation. Request ID, structured logging, and panic recovery middleware apply to every route.

## Seed

Create the example backlog. Re-running the command does not duplicate issues.

```sh
docker compose run --rm api seed
```

Seed issues belong to `VIAL_SEED_OWNER_USERNAME`, which must name an existing user.

## Test

Start PostgreSQL and run the integration test against it:

```sh
docker compose up -d db
TEST_DATABASE_URL='postgres://vialboard:vialboard@localhost:5432/vialboard?sslmode=disable' go test ./...
```

The Compose credentials are for local development only.

## Migrations

Migrations are embedded in the application and applied automatically at startup.

```sh
docker compose run --rm api migrate up
docker compose run --rm api migrate down
```

`migrate down` rolls back the latest migration and may delete data.

Create schema changes as numbered SQL files in `internal/infrastructure/postgres/migrations/`.

## Operations

Production Compose, backups, restore instructions, health checks, and initial service objectives are documented in [`docs/operations.md`](docs/operations.md). Copy `.env.example` to `.env` and replace every placeholder before using `compose.prod.yaml`.
