# Course Service

Micro-service that serves course/module content for the e-learning platform.

## Architecture

- **Source of truth** — courses, modules and learning paths live in PostgreSQL and are managed through the admin API. The service holds no course data in memory: every request reads what it needs from the database, so any replica serves the same catalogue and a restart or rollout loses nothing.
- **Reads are sized to the request** — catalogue listings filter in SQL and never touch module rows; rendering a course reads its modules in one query indexed on `(course_slug, position)`.
- **A database is required** — the service refuses to start without `DATABASE_URL`. See [Database](#database) below.
- **Module types** — `text` (markdown from git), `video` / `image` (server-hosted URLs with optional replication), `quiz` (inline questions or git-fetched YAML).
- **User Service calls** — enrollment checks, lesson progress (viewed, complete) are delegated to the User Service via HTTP:
  - `GET /internal/enrollments/check` — enrollment check
  - `GET /internal/progress/viewed` — viewed lessons set
  - `POST /internal/progress/complete` — mark lesson complete

## API Endpoints

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/health` | No | Liveness check |
| GET | `/metrics` | No | Prometheus metrics |
| GET | `/api/courses` | No | List published courses |
| GET | `/api/courses/{slug}` | No | Get single course |
| GET | `/api/courses/{slug}/modules` | JWT | List modules with viewed status |
| GET | `/api/courses/{slug}/modules/{index}` | JWT | Get module with content (inline quiz questions for quiz type) |
| POST | `/api/courses/{slug}/modules/{index}/submit` | JWT | Submit quiz answers for a quiz-type module |
| GET | `/api/courses/{slug}/labs` | JWT | List labs |
| GET | `/api/courses/{slug}/labs/{lab_id}` | JWT | Get lab detail with progress |
| GET | `/api/courses/{slug}/progress` | JWT | Get course progress |
| GET | `/api/courses/{slug}/lessons` | JWT | List lessons with viewed status |
| GET | `/api/courses/{slug}/lessons/{lessonSlug}` | JWT | Get lesson content |
| POST | `/api/courses/{slug}/lessons/{lessonSlug}/complete` | JWT | Mark lesson complete |
| POST | `/api/admin/cache/clear` | JWT+Admin | Clear git cache (force re-clone on next access) |
| GET | `/api/admin/courses` | JWT+Admin | List all courses including private ones |
| POST | `/api/admin/courses` | JWT+Admin | Create a course from `{slug, spec}` |
| GET | `/api/admin/courses/{slug}/definition` | JWT+Admin | Get a course's full definition |
| PUT | `/api/admin/courses/{slug}/definition` | JWT+Admin | Replace a course's definition |
| DELETE | `/api/admin/courses/{slug}/definition` | JWT+Admin | Delete a course |
| POST | `/api/admin/courses/paths` | JWT+Admin | Create a learning path from `{slug, spec}` |
| PUT | `/api/admin/courses/paths/{slug}/definition` | JWT+Admin | Replace a path's definition |
| DELETE | `/api/admin/courses/paths/{slug}/definition` | JWT+Admin | Delete a learning path |
| GET | `/uploads/{filename}` | No | Serve uploaded media |

## Configuration

All config via environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `8082` | HTTP listen port |
| `JWT_SECRET` | `change-me-...` | HMAC key for JWT tokens |
| `JWT_EXPIRY_HOURS` | `24` | Token TTL |
| `CORS_ORIGINS` | `http://localhost:3000,http://localhost:5173` | Allowed CORS origins |
| `UPLOADS_DIR` | `./uploads` | Directory for uploaded media |
| `USER_SERVICE_URL` | `http://localhost:8081` | Base URL of User Service |
| `GIT_TOKEN` | (empty) | Global token for private git repos (fallback) |
| `GIT_CREDENTIALS_PATH` | `/etc/course-service/git-credentials.yaml` | Path to per-repo credential mappings |
| `GIT_CACHE_TTL` | `10` | Git cache TTL in minutes (how long before re-cloning remote repos) |
| `DATABASE_URL` | (required) | PostgreSQL connection string; see [Database](#database) |
| `DB_MAX_OPEN_CONNS` | `10` | Maximum open database connections in the pool |
| `DB_MAX_IDLE_CONNS` | `10` | Maximum idle database connections in the pool |
| `SEED_DEV_COURSES` | (unset) | `true` seeds the demo catalogue's missing courses on startup, `overwrite` replaces them; anything else disables seeding. See [Dev seed](#dev-seed) |

## Database

`DATABASE_URL` is required: the catalogue itself lives in Postgres, so there is nothing to serve
without it. Startup retries the initial connection a bounded number of times before giving up, so
booting alongside a Postgres pod that is not ready yet is a slow start rather than a crash loop.

Uses PostgreSQL via [GORM](https://gorm.io), with the same `AutoMigrate` + one-off breaking-migration
split as User Service — see `internal/db/db.go`.

### Schema

| Table | Contents |
|---|---|
| `courses` | Course metadata; `category`, `difficulty`, `public` and `title` are indexed for catalogue filtering, and `skills` is denormalized from the modules |
| `course_modules` | One row per module, ordered by `position`, unique on `(course_slug, position)`; the opaque leaf payloads (`questions`, `steps`, `check_params`) are `jsonb` |
| `course_prerequisites` | Enrollment conditions |
| `course_sessions` | Scheduled in-person sessions, keyed `(course_slug, session_id)` so a retried write overwrites the same row |
| `paths`, `path_courses`, `path_skills` | Learning paths and their ordered members |
| `quiz_question_attempts` | Per-question attempt count and cooldown deadline; persisted so retry backoff survives restarts and holds across replicas |
| `lab_checks` | Recorded outcome of a lab module check (server-verified or client-reported) |

## Dev seed

A demo catalogue of 17 courses lives in `internal/db/seed/courses/` and is
embedded in the binary, so seeding works identically in a KinD cluster and on
a laptop. It is loaded on startup only when `SEED_DEV_COURSES` is set:

| Value | Behaviour |
|---|---|
| `true` | Creates only the courses that do not exist yet, so local edits survive a restart |
| `overwrite` | Replaces every seed course — use after editing the seed files |
| anything else (default) | No seeding |

Each file is a course definition in the same shape the admin API accepts,
with the slug at the top level:

```yaml
slug: linux-intro
spec:
  title: "Introduction à Linux"
  category: linux
  difficulty: beginner
  public: true
  modules:
    - name: "Navigation et système de fichiers"
      type: text
      content: |
        ## Navigation …
```

Seeding never touches a course that is not part of the seed set.

## Git Credentials

For `text` and `quiz` modules that reference a private git repo (`src` + `ref` + `path`), the service needs a token to authenticate.

### Global token (all repos)

```sh
export GIT_TOKEN=ghp_xxxxxxxxxxxx
```

### Per-repo tokens (mount a Secret as volume)

Create a Secret with a `git-credentials.yaml` key:

```yaml
credentials:
  - url: "github.com/myorg/*"
    token: "ghp_xxx"
  - url: "gitlab.com/other/*"
    token: "glpat_yyy"
```

Apply it:

```sh
kubectl create secret generic course-repo-secret \
  --from-file=git-credentials.yaml=./git-credentials.yaml
```

The Helm chart mounts this secret at `GIT_CREDENTIALS_PATH` automatically.
URL matching uses glob patterns (`path.Match`). The first match wins.
If no credential matches, `GIT_TOKEN` is used as a fallback.
If both are empty, the clone is unauthenticated (public repos only).

## Resource Replication

When a module has `replication: true` and type is `video` or `image`, the service
downloads the remote resource and caches it locally in the `UPLOADS_DIR`. The API
then returns the local `/uploads/` URL instead of the remote URL.

This is useful for:

- Reducing external dependency (resource served even if remote goes down)
- Improving load times (no redirect to external CDN)
- Air-gapped deployments

Replication has no effect on `text` modules.

```yaml
modules:
  - name: "Architecture Overview"
    type: "video"
    src: "https://example.com/videos/k8s-arch.mp4"
    replication: true   # ← cached locally at /uploads/<hash>.mp4
```

## Quiz Modules

Modules with `type: "quiz"` support inline questions or git-fetched quiz YAML.

### Inline questions

```yaml
modules:
  - name: "K8s Basics Quiz"
    type: "quiz"
    passingScore: 80
    questions:
      - id: "q1"
        type: "single"
        points: 1
        question: "What is a Pod?"
        answers:
          - id: "a"
            text: "Smallest deployable unit"
            correct: true
```

### Git-fetched quiz

Use `src`, `ref`, and `path` to reference a YAML file in a git repo (same as `text` modules):

```yaml
  - name: "K8s Basics Quiz"
    type: "quiz"
    src: "https://github.com/user/repo"
    ref: "main"
    path: "quizzes/kubernetes-basics.yaml"
```

### Scoring

Supported question types: `single` (radio), `multiple` (checkbox with optional partial scoring), `boolean` (true/false), `order` (drag-to-rank). Results include per-question feedback, correct answers, and source references.

## How to Run

### Locally

```sh
DATABASE_URL=postgres://pupitre:pupitre@localhost:5432/pupitre go run main.go
```

Requires a reachable PostgreSQL instance; the schema is migrated automatically on startup.

### Docker

```sh
docker build -t course-service .
docker run -p 8082:8082 -e DATABASE_URL=postgres://pupitre:pupitre@postgres:5432/pupitre course-service
```

### Kubernetes

Deploy as a standard Deployment. No Kubernetes API access is needed — the pods run without a mounted service-account token.

## Development Workflow (KinD)

### Quick rebuild after code changes

```sh
# From project root — build, load, restart in one command
make rebuild-course

# Or manually:
docker build -t localhost/pupitre-course-service:latest course-service/
kind load docker-image localhost/pupitre-course-service:latest --name pupitre
kubectl rollout restart deploy/pupitre-course-service

# Check logs
make logs
```

### Local testing (without Helm/KinD)

```sh
# Prerequisites: a reachable PostgreSQL instance
cd course-service
DATABASE_URL=postgres://pupitre:pupitre@localhost:5432/pupitre go run .
```

## Dependencies

- **PostgreSQL** reachable via `DATABASE_URL`
- **User Service** running and accessible via `USER_SERVICE_URL`
