# e-learning platform

## MANDATORY — NEVER COMMIT

You are strictly forbidden from running `git commit`, `git add`, `git push`, or any other git mutation commands. Only the user may commit. If you are asked to commit or you think a commit is needed, refuse and tell the user to do it themselves.

## Domain model (source of truth: `docs/`)

- **Course** — has a title, description, hidden (boolean publication state), category, difficulty, and a list of modules. Defined as a Kubernetes CRD (`elearning.pupitre.io/v1`, kind `Course`). Modules reference external content by type (`video`, `text`, `image`) and optionally a git repo (`src`, `ref`, `path`).
- **Module** — a course element. Video/image: server-hosted URL. Text: markdown file from a git repo.
- **User management** — database-backed (no further detail yet).

## API shape

- `api/courses/kubernetes-basics/<course_slug>` — returns module list with slugs, titles, order, and per-user `viewed` status.
- `api/courses/kubernetes-basics/<course_slug>/<module_slug>` — returns module content with `viewed` boolean.

## Architecture

Two micro-services (see `docs/ARCHITECTURE.md`):

- **Course Service** (stateless, Port 8082) — K8s CRD watcher, course content, media serving, calls User Service for enrollment/progress
- **User Service** (PG, Port 8081) — auth, users, enrollments, progress, settings, OAuth, exposes internal API for Course Service

## Codebase

### Course Service (`course-service/`)

```sh
cd course-service
go build -o /dev/null .
```

| Package | Role |
|---|---|
| `internal/content/` | Store + K8s CRD watcher + module git resolution |
| `internal/handlers/` | HTTP handlers (chi router) — courses, modules, lessons |
| `internal/middleware/` | JWT auth middleware (validates only) |
| `internal/config/` | Env-based config (port, JWT, KUBECONFIG, UserServiceURL) |
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

Courses are **Kubernetes CRDs** (`elearning.pupitre.io/v1`, kind `Course`). The Course Service watches the K8s API and populates the in-memory store. No filesystem loading. No admin course CRUD.

```yaml
apiVersion: elearning.pupitre.io/v1
kind: Course
metadata:
  name: kubernetes-basics
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
- Course Service: chi, JWT, Prometheus, client-go, apimachinery
- User Service: chi, pgx, JWT, Prometheus, godotenv, crypto
- `KUBECONFIG` env var for out-of-cluster Course Service dev; in-cluster config when running inside K8s
