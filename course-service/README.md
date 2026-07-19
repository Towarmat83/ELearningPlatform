# Course Service

Stateless micro-service that serves course/module content for the e-learning platform.

## Architecture

- **Course content is stateless** — all course data comes from Kubernetes CRDs (`elearning.pupitre.io/v1`, kind `Course`) via an in-cluster watcher; nothing course-related is persisted to a database.
- **Lab results are optionally persisted** — see [Database](#database) below.
- **Source of truth** — courses are defined as Kubernetes custom resources. The service watches the K8s API and populates an in-memory store.
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
| `KUBECONFIG` | (empty) | Path to kubeconfig (out-of-cluster); uses in-cluster config when empty |
| `K8S_NAMESPACE` | `default` | Namespace for Course CRDs |
| `USER_SERVICE_URL` | `http://localhost:8081` | Base URL of User Service |
| `GIT_TOKEN` | (empty) | Global token for private git repos (fallback) |
| `GIT_CREDENTIALS_PATH` | `/etc/course-service/git-credentials.yaml` | Path to per-repo credential mappings |
| `GIT_CACHE_TTL` | `10` | Git cache TTL in minutes (how long before re-cloning remote repos) |
| `DATABASE_URL` | (empty) | PostgreSQL connection string; see [Database](#database) |
| `DB_MAX_OPEN_CONNS` | `10` | Maximum open database connections in the pool |
| `DB_MAX_IDLE_CONNS` | `10` | Maximum idle database connections in the pool |

## Database

Course content itself is never persisted (see [Architecture](#architecture)), but lab check results
are, when `DATABASE_URL` is set. If it's empty, or the initial connection fails, the service logs a
warning and keeps running with lab result tracking disabled (`GET`/`POST` on `/api/admin/lab-results`
degrade rather than fail the whole service) — connectivity is only checked once at startup, so a
Postgres instance that isn't ready yet at boot time stays disabled until the pod restarts.

Uses PostgreSQL via [GORM](https://gorm.io), with the same `AutoMigrate` + one-off breaking-migration
split as User Service — see `internal/db/db.go`.

### Schema

- `lab_checks` — recorded outcome of a lab module check (server-verified or client-reported)

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
go run main.go
```

Requires a K8s cluster with the `elearning.pupitre.io/v1` Course CRD installed and at least one Course resource.

### Docker

```sh
docker build -t course-service .
docker run -p 8082:8082 -e KUBECONFIG=/app/kubeconfig -v /path/to/kubeconfig:/app/kubeconfig course-service
```

### Kubernetes

Deploy as a standard Deployment. The service will use in-cluster config to watch Course CRDs.

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
# Prerequisites: KinD cluster running + CRD installed
cd course-service
go run . -kubeconfig ~/.kube/config
```

## Dependencies

- **Kubernetes cluster** with `elearning.pupitre.io/v1` Course CRD installed
- **User Service** running and accessible via `USER_SERVICE_URL`
