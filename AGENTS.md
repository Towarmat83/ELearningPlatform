# e-learning platform

## MANDATORY — NEVER COMMIT

You are strictly forbidden from running `git commit`, `git add`, `git push`, or any other git mutation commands. Only the user may commit. If you are asked to commit or you think a commit is needed, refuse and tell the user to do it themselves.

## Domain model (source of truth: `docs/`)

- **Course** — has a title, description, hidden (boolean publication state), category, difficulty, and a list of modules. Stored in PostgreSQL and managed through the course-service admin API. Modules reference external content by type (`video`, `text`, `image`) and optionally a git repo (`src`, `ref`, `path`).
- **Module** — a course element. Video/image: server-hosted URL. Text: markdown file from a git repo.
- **User management** — database-backed (no further detail yet).

## API shape

- `api/courses/kubernetes-basics/<courseSlug>` — returns module list with slugs, titles, order, and per-user `viewed` status.
- `api/courses/kubernetes-basics/<courseSlug>/<moduleSlug>` — returns module content with `viewed` boolean.

## Architecture

Two micro-services (see `docs/ARCHITECTURE.md`):

- **Course Service** (PG, Port 8082) — course catalogue and content, media serving, calls User Service for enrollment/progress
- **User Service** (PG, Port 8081) — auth, users, enrollments, progress, settings, OAuth, exposes internal API for Course Service

## Codebase

### Course Service (`course-service/`)

```sh
cd course-service
go build -o /dev/null .
```

| Package | Role |
|---|---|
| `internal/content/` | Domain types, quiz scoring, git module resolution |
| `internal/models/` + `internal/repository/` | GORM models and data access (courses, modules, paths, quiz attempts, lab checks) |
| `internal/db/` | PG connection + AutoMigrate + dev course seed |
| `internal/definition/` | Wire shape of a course/path definition, shared by the admin API and the seeder |
| `internal/handlers/` | HTTP handlers (chi router) — courses, modules, lessons |
| `internal/middleware/` | JWT auth middleware (validates only) |
| `internal/config/` | Env-based config (port, JWT, DATABASE_URL, UserServiceURL) |
| `internal/metrics/` | Prometheus metrics |

**Routes:** Public: `/health`, `/metrics`, `/api/courses`, `/api/courses/{slug}`, `/uploads/{filename}` — Authenticated: `/api/courses/{slug}/modules`, `/api/courses/{slug}/lessons`, `/api/courses/{slug}/lessons/{lesson}/complete`

### User Service (`user-service/`)

```sh
cd user-service
go build -o /dev/null .
```

| Package | Role |
|---|---|
| `internal/db/` | PG connection + migration runner |
| `internal/handlers/` | HTTP handlers — auth, oauth, settings, admin, enrollments, progress |
| `internal/middleware/` | JWT auth middleware (create + validate) |
| `internal/config/` | Env-based config (DB, JWT, OAuth) |
| `internal/metrics/` | Prometheus metrics |
| `migrations/` | SQL migrations (embed) |

**Routes:** Public auth + OAuth, Authenticated (enroll, progress, my courses), Admin (users, settings, enrollments), Internal (for Course Service): `/internal/enrollments/check`, `/internal/progress/viewed`, `/internal/progress/complete`

## Course source of truth

Courses live in **PostgreSQL** and are managed through the admin API. There is no in-memory store, no filesystem loading, and no Kubernetes controller: every request reads what it needs from the database.

```
POST   /api/admin/courses                      # create  {slug, spec}
GET    /api/admin/courses/{slug}/definition    # read
PUT    /api/admin/courses/{slug}/definition    # replace {spec}
DELETE /api/admin/courses/{slug}/definition    # delete
```

The `spec` body (shown as YAML for readability; the API takes JSON):

```yaml
slug: kubernetes-basics
spec:
  title: "Kubernetes Basics"
  description: "..."
  hidden: false
  category: "kubernetes"
  difficulty: "beginner"
  modules:
    - name: "What is K8s"
      type: "text"
      src: "https://github.com/user/repo"
      ref: "main"
      path: "lessons/intro.md"
```

## Build notes

- Go 1.26
- Course Service: chi, JWT, Prometheus, GORM/pgx, go-git
- User Service: chi, pgx, JWT, Prometheus, godotenv, crypto
- `DATABASE_URL` is required by both services; course-service refuses to start without it
- `SEED_DEV_COURSES=true` seeds the embedded demo catalogue (`course-service/internal/db/seed/courses/`); `overwrite` replaces it. Unset in production.
