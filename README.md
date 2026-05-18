# Go Task Management Server

> Production-patterned Go REST API — JWT auth, SQLite persistence, task CRUD with tag filtering, and a full `httptest`-based test suite.

---

## Features

- **JWT authentication** — register and login return signed tokens; all task routes are protected
- **Task CRUD** — create, read, update, and delete tasks scoped to the authenticated user
- **Tag management** — add and remove tags per task; filter your task list by tag
- **Status lifecycle** — `todo` → `in-progress` → `done`, with an overdue filter
- **Due dates** — optional ISO-8601 due date with `?overdue=true` filtering
- **Ownership isolation** — every query is scoped by `user_id`; users cannot access each other's data
- **Persistent storage** — SQLite via `modernc.org/sqlite` (pure Go, no CGo required)
- **Zero-config** — sensible env var defaults so the binary runs immediately with no setup
- **Full test suite** — integration tests using `net/http/httptest` and in-memory SQLite; no mocks

---

## Project Structure

```
go-tm-server/
├── main.go
├── go.mod
└── internal/
    ├── db/          # schema init and SQLite connection
    ├── models/      # User, Task, Status types
    ├── middleware/  # JWT auth + request logging
    ├── handlers/    # auth and task HTTP handlers
    ├── server/      # router and middleware composition
    └── testutil/    # shared test helpers and integration tests
```

---

## Getting Started

**Prerequisites:** [Go 1.22+](https://go.dev/dl/)

```bash
git clone https://github.com/js-carpadakis/go-tm-server
cd go-tm-server
go mod tidy
go run main.go
```

The server starts on `:8080` with a `tasks.db` SQLite file created automatically.

---

## Configuration

All configuration is via environment variables with built-in fallbacks.

| Variable | Default | Description |
|---|---|---|
| `DATABASE_URL` | `tasks.db` | SQLite file path. Use `:memory:` for ephemeral. |
| `JWT_SECRET` | `dev-secret-change-in-production` | HMAC secret for signing tokens. **Change this in production.** |
| `ADDR` | `:8080` | TCP address the server listens on. |

```bash
JWT_SECRET=supersecret ADDR=:9000 go run main.go
```

---

## API Reference

### Auth

| Method | Path | Auth | Description |
|---|---|---|---|
| `POST` | `/auth/register` | — | Create account, receive JWT |
| `POST` | `/auth/login` | — | Verify credentials, receive JWT |

**Request body** (both endpoints):
```json
{ "email": "you@example.com", "password": "password123" }
```

**Response:**
```json
{ "token": "<jwt>" }
```

---

### Tasks

All task endpoints require `Authorization: Bearer <token>`.

| Method | Path | Description |
|---|---|---|
| `GET` | `/tasks` | List your tasks (supports filters) |
| `POST` | `/tasks` | Create a task |
| `GET` | `/tasks/{id}` | Get a single task |
| `PUT` | `/tasks/{id}` | Update a task (partial — omitted fields unchanged) |
| `DELETE` | `/tasks/{id}` | Delete a task |
| `POST` | `/tasks/{id}/tags` | Add tags to a task |
| `DELETE` | `/tasks/{id}/tags/{tag}` | Remove a tag from a task |

**Query filters for `GET /tasks`:**

| Param | Example | Description |
|---|---|---|
| `status` | `?status=todo` | Filter by status (`todo`, `in-progress`, `done`) |
| `tag` | `?tag=work` | Filter to tasks with this tag |
| `overdue` | `?overdue=true` | Tasks past their due date and not done |

**Task object:**
```json
{
  "id": 1,
  "user_id": 3,
  "title": "Buy milk",
  "description": "Semi-skimmed",
  "status": "todo",
  "due_date": "2026-05-20T09:00:00Z",
  "tags": ["shopping", "urgent"],
  "created_at": "2026-05-18T14:00:00Z",
  "updated_at": "2026-05-18T14:00:00Z"
}
```

---

## Walkthrough

```bash
# 1. Register and capture the token
TOKEN=$(curl -s -X POST http://localhost:8080/auth/register \
  -H "Content-Type: application/json" \
  -d '{"email":"me@example.com","password":"password123"}' | jq -r .token)

# 2. Create a task with tags
curl -s -X POST http://localhost:8080/tasks \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"title":"Buy milk","tags":["shopping"],"status":"todo"}' | jq

# 3. List tasks filtered by tag
curl -s "http://localhost:8080/tasks?tag=shopping" \
  -H "Authorization: Bearer $TOKEN" | jq

# 4. Mark it done
curl -s -X PUT http://localhost:8080/tasks/1 \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"status":"done"}' | jq

# 5. Delete it
curl -s -X DELETE http://localhost:8080/tasks/1 \
  -H "Authorization: Bearer $TOKEN"
```

---

## Running Tests

```bash
go test ./... -v
```

Each test gets a fresh in-memory SQLite database — no teardown, no test pollution, no Docker required.

---

## Dependencies

| Package | Purpose |
|---|---|
| [`github.com/golang-jwt/jwt/v5`](https://github.com/golang-jwt/jwt) | JWT signing and verification |
| [`golang.org/x/crypto`](https://pkg.go.dev/golang.org/x/crypto) | bcrypt password hashing |
| [`modernc.org/sqlite`](https://pkg.go.dev/modernc.org/sqlite) | Pure-Go SQLite driver (no CGo) |

Everything else is Go standard library.
