# TaskFlow — Backend

A production-ready REST API for task management, built with **Go**, **PostgreSQL**, and **Docker**.

---

## 1. Overview

TaskFlow is a task management system where users can register, create projects, add tasks, and assign work to team members. This repository contains the **backend only** (as required for the Backend Engineer track).

**Tech stack:**
- **Language:** Go 1.22
- **Router:** [chi](https://github.com/go-chi/chi) — lightweight, idiomatic, and stdlib-compatible
- **Database:** PostgreSQL 16
- **Auth:** JWT (HS256, 24h expiry) via `golang-jwt/jwt/v5`
- **Password hashing:** `bcrypt` at cost 12
- **Migrations:** `golang-migrate/migrate` (file-based, auto-run on startup)
- **Logging:** `log/slog` (structured JSON)
- **Docker:** multi-stage build → ~20MB final image

---

## 2. Architecture Decisions

### Package structure
```
cmd/server/         — entry point, wires everything together
internal/
  config/           — env-based config loading
  db/               — pool creation + migration runner
  handler/          — HTTP handlers (auth, projects, tasks)
  handler/response/ — shared JSON response helpers
  middleware/        — JWT auth middleware
  model/            — request/response types and domain structs
migrations/         — numbered up/down SQL migration files
tests/              — integration tests
```

`internal/` enforces that no external package can import these — clean boundary between the HTTP layer and business logic.

### Why chi over Gin/Echo?
Chi uses `net/http` directly, has no external dependencies beyond stdlib, compiles to a tiny binary, and provides all the routing features needed (path params, middleware groups, method routing). Gin adds ~10 extra transitive dependencies for marginal benefit at this scale.

### Pagination
All list endpoints support `?page=&limit=` (default: page 1, limit 20, max 100). Responses return `total_count` and `total_pages` so clients can build pagination UI without extra round-trips.

### Access control model
- A **project owner** has full CRUD on the project and its tasks.
- A user with **any task assigned** in a project can view that project and its tasks, and update/delete those tasks.
- This mirrors real-world team collaboration without requiring a full RBAC system at this scope.

### What I intentionally left out
- **Refresh tokens** — the 24h JWT expiry is suitable for this scope; refresh tokens would add significant complexity.
- **Rate limiting** — important for production but out of scope; the middleware stub is easy to add.
- **Email verification** — adds infrastructure (SMTP/SES) without adding signal in this assessment context.
- **Role-based access control** — the owner/assignee model covers the spec without a roles table.

---

## 3. Running Locally

> **Prerequisites:** Docker and Docker Compose. Nothing else needed.

```bash
# 1. Clone the repo
git clone https://github.com/your-name/taskflow-backend
cd taskflow-backend

# 2. Create your .env from the example
cp .env.example .env

# 3. Set a real JWT secret in .env
#    (or leave the default for local dev)

# 4. Start everything — postgres, migrations, seed data, and the API
docker compose up --build

# API is now running at:
#   http://localhost:8080
```

To run in the background:
```bash
docker compose up --build -d
docker compose logs -f api
```

To tear down and remove volumes:
```bash
docker compose down -v
```

---

## 4. Running Migrations

Migrations run **automatically** when the API container starts (via `golang-migrate`). The `migrate` service in `docker-compose.yml` runs before the `api` service thanks to `depends_on` + healthcheck ordering.

To run manually against a local Postgres instance:
```bash
# Install migrate CLI
go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest

# Run up
migrate -path ./migrations \
        -database "postgres://postgres:postgres@localhost:5432/taskflow?sslmode=disable" \
        up

# Roll back one step
migrate -path ./migrations \
        -database "postgres://postgres:postgres@localhost:5432/taskflow?sslmode=disable" \
        down 1
```

---

## 5. Test Credentials

The seed script creates two users automatically:

| Name       | Email               | Password    |
|------------|---------------------|-------------|
| Test User  | test@example.com    | password123 |
| Jane Smith | jane@example.com    | password123 |

And one seeded project ("Website Redesign") with 3 tasks in different statuses.

---

## 6. API Reference

### Base URL
```
http://localhost:8080
```

### Auth

#### `POST /auth/register`
```json
// Request
{ "name": "Alice", "email": "alice@example.com", "password": "password123" }

// Response 201
{ "token": "<jwt>", "user": { "id": "uuid", "name": "Alice", "email": "alice@example.com", "created_at": "..." } }
```

#### `POST /auth/login`
```json
// Request
{ "email": "alice@example.com", "password": "password123" }

// Response 200
{ "token": "<jwt>", "user": { ... } }
```

All endpoints below require:
```
Authorization: Bearer <token>
```

---

### Projects

#### `GET /projects?page=1&limit=20`
Returns projects where you are the owner or have tasks assigned.
```json
{
  "data": [ { "id": "uuid", "name": "...", "owner_id": "uuid", "created_at": "..." } ],
  "page": 1, "limit": 20, "total_count": 3, "total_pages": 1
}
```

#### `POST /projects`
```json
// Request
{ "name": "My Project", "description": "Optional" }
// Response 201 — project object
```

#### `GET /projects/:id`
Returns project details + its tasks.
```json
{
  "id": "uuid", "name": "...", "tasks": [
    { "id": "uuid", "title": "...", "status": "todo", "priority": "high", ... }
  ]
}
```

#### `PATCH /projects/:id`  _(owner only)_
```json
{ "name": "New Name", "description": "New desc" }
// Response 200 — updated project object
```

#### `DELETE /projects/:id`  _(owner only)_
```
Response 204 No Content
```

#### `GET /projects/:id/stats`
```json
{
  "project_id": "uuid",
  "by_status": { "todo": 2, "in_progress": 1, "done": 0 },
  "by_assignee": [ { "assignee_id": "uuid", "assignee_name": "Alice", "count": 2 } ]
}
```

---

### Tasks

#### `GET /projects/:id/tasks?status=todo&assignee=<uuid>&page=1&limit=20`
Supports optional `status` and `assignee` query filters plus pagination.

#### `POST /projects/:id/tasks`
```json
{
  "title": "Design homepage",
  "description": "Optional",
  "priority": "high",
  "assignee_id": "uuid (optional)",
  "due_date": "2026-05-01 (optional, YYYY-MM-DD)"
}
// Response 201 — task object (status defaults to "todo")
```

#### `PATCH /tasks/:id`
All fields optional:
```json
{ "title": "...", "status": "in_progress", "priority": "low", "assignee_id": "uuid", "due_date": "2026-05-10" }
// Response 200 — updated task object
```

#### `DELETE /tasks/:id`  _(project owner or assigned member)_
```
Response 204 No Content
```

---

### Error Responses

```json
// 400 Validation
{ "error": "validation failed", "fields": { "email": "is required" } }

// 401 Unauthenticated
{ "error": "unauthorized" }

// 403 Forbidden
{ "error": "forbidden" }

// 404 Not found
{ "error": "not found" }

// 500 Internal
{ "error": "internal server error" }
```

---

## 7. Running Tests

Integration tests require a running PostgreSQL instance:

```bash
# Spin up just postgres
docker compose up -d postgres

# Run tests
TEST_DATABASE_URL="postgres://postgres:postgres@localhost:5432/taskflow?sslmode=disable" \
  go test ./tests/... -v
```

The tests cover:
- Register (success, duplicate email, validation errors)
- Login (success, wrong password)
- Protected routes without token → 401
- Project create, get, update, delete
- Cross-user forbidden access
- Task create, list, update (status change), delete
- Task filter by status

---

## 8. What I'd Do With More Time

**Missing or deferred:**
- **Refresh tokens** with a token store (Redis) and `/auth/refresh` endpoint
- **Rate limiting** per IP and per user on auth endpoints (e.g. `golang.org/x/time/rate`)
- **More integration test coverage** — currently ~80% of happy paths, missing some edge cases on cascade deletes and concurrent updates
- **WebSocket / SSE** for real-time task status updates (already a clean integration point at the task PATCH handler)
- **OpenAPI / Swagger spec** auto-generated from handler annotations
- **Soft deletes** (`deleted_at` column) so data isn't permanently lost on accidental deletes
- **Audit log table** — `task_id`, `changed_by`, `field`, `old_value`, `new_value`, `changed_at`

**Shortcuts taken:**
- The `PATCH` update queries use `CASE WHEN $n::boolean` instead of a query builder — this works correctly but a proper `squirrel` or `pgx/v5/pgxutil` dynamic builder would be cleaner at scale
- CORS is wide-open (`*`) by default — fine for dev, `CORS_ORIGIN` env var narrows it in production
- No request body size limit middleware — should add `http.MaxBytesReader` in production
