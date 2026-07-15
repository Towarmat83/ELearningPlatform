# E-Learning Platform

Micro-services e-learning platform with Kubernetes CRD-based course definitions and automated lab validation.

## Architecture

```mermaid
graph LR
    subgraph Clients["Clients"]
        Browser(["Browser"])
        Pupitre(["Pupitre\n(Tauri desktop)"])
    end

    subgraph Platform["Platform — Kubernetes"]
        Frontend["Frontend\nAstro SSR · :3000"]

        subgraph Services["Backend Services"]
            UserService["User Service :8081\nAuth · Enrollments · Progress"]
            CourseService["Course Service :8082\nCourses · Labs · Quiz"]
            CheckerService["Checker Service :8083\nOPA/Rego · GitLab fetch"]
        end

        subgraph Data["Data"]
            PostgreSQL[("PostgreSQL")]
            K8sCRD[["K8s CRD\nCourse definitions"]]
            GitRepos[["Git repos\nLab content"]]
        end
    end

    subgraph External["External"]
        GitLab["GitLab\nStudent projects"]
        OAuth["OAuth2 / OIDC\nKeycloak · GitHub…"]
        Podman["Podman\nLocal machine"]
    end

    Browser -->|HTTP| Frontend
    Pupitre -->|WebView| Frontend
    Pupitre -->|"local_check\n(Rust command)"| Podman

    Frontend --> UserService
    Frontend --> CourseService

    CourseService -->|Internal API| UserService
    CourseService -->|"POST /evaluate\n(GitLab labs)"| CheckerService

    UserService --> PostgreSQL
    CourseService --> PostgreSQL
    CourseService --> K8sCRD
    CourseService --> GitRepos
    CheckerService --> GitRepos
    CheckerService --> GitLab
    UserService -.->|OIDC| OAuth
```

| Service | Role | Tech |
|---------|------|------|
| **Course Service** | Course content, media, quiz, inline lab rendering, checker proxy, lab_checks persistence | Go + chi + client-go + pgx |
| **User Service** | Auth (local · OAuth2/OIDC), enrollments, progress, admin | Go + chi + pgx |
| **Checker Service** | Fetch live GitLab state, evaluate OPA/Rego policy, return violations | Go + chi + go-gitlab + OPA |
| **Frontend** | Astro SSR, API proxy, markdown lab rendering, admin Labs page | Astro |

## Source of truth

Courses are defined as **Kubernetes CRDs** (`elearning.pupitre.io/v1`, kind `Course`). See [`docs/Course.md`](docs/Course.md) for the full spec.

## Interactive Labs — Lab Checker

Lab modules (`type: lab`) display the assignment inline (markdown) and let students validate their GitLab work via the **"Vérifier mon travail"** button.

Each lab requires two files co-located with `content.md` in the git repo:

```
modules/lab1/
  ├── content.md    # Assignment (markdown)
  ├── check.yaml    # Checker config: provider, project template, files to verify
  └── check.rego    # OPA/Rego policy: validation rules
```

Validation flow:

```
Student clicks "Vérifier"
  → course-service reads check.yaml + check.rego from git
  → POST checker-service /evaluate
  → checker fetches live GitLab state (MRs, commits, pipeline, files)
  → evaluates Rego → {allow, violations}
  → stored in DB (lab_checks)
  → result displayed to student
```

Instructors can review all attempts at `/admin/labs`.

See [`docs/interactive-labs.md`](docs/interactive-labs.md) for full documentation.

## Pupitre — Desktop App (Local Labs)

Some labs require verifying work done locally on the student's machine (e.g. `podman pull`, `podman run`). For these, the platform runs as a **Tauri v2 desktop app** called **Pupitre**.

```
tauri-app/
├── src/              # Minimal HTML shell (loads localhost:3000)
└── src-tauri/        # Rust backend
    └── src/lib.rs    # Local check commands (podman images, podman events)
```

When running inside Pupitre, the "Vérifier mon travail" button calls a local Rust command instead of the remote checker-service:

```
Student clicks "Vérifier" (inside Pupitre)
  → frontend detects window.__TAURI_INTERNALS__
  → invoke("local_check", { checkType, params })
  → Rust runs podman commands locally
  → {allow, violations} returned to frontend
```

Lab modules with `checkProvider: local` in the CRD use this flow. Labs with `checkProvider: gitlab` (or no provider) use the remote checker-service as before.

### Build Pupitre

```bash
# macOS (ARM64)
cd tauri-app/src-tauri && cargo build --release

# Linux x86_64 (via Silverblue VM or toolbox)
toolbox run bash -c 'source ~/.cargo/env && cd ~/tauri-app/src-tauri && cargo build --release'
```

## Quick start

```bash
make dev
```

See `CONTRIBUTING.md` for the full step-by-step guide, troubleshooting, and deployment details.

## Project structure

```
./
├── checker-service/      # Go service (port 8083)
│   └── internal/
│       ├── checker/      # GitLab fetch + Rego evaluation
│       ├── handlers/     # POST /evaluate
│       └── config/       # Env config
├── course-service/       # Go service (port 8082)
│   ├── internal/
│   │   ├── content/      # Store, K8s watcher, git fetch, quiz scoring
│   │   ├── db/           # PG connect + migrations
│   │   ├── handlers/     # HTTP routes + CheckModule + lab_results
│   │   ├── middleware/   # JWT auth
│   │   ├── config/       # Env config
│   │   └── metrics/      # Prometheus
│   └── migrations/       # Embedded SQL migrations
├── user-service/         # Go service (port 8081)
│   ├── internal/
│   │   ├── db/           # PG + migrations
│   │   ├── handlers/     # Auth, OAuth, admin, progress
│   │   ├── middleware/   # JWT
│   │   └── config/       # Env config
│   └── migrations/       # Embedded SQL migrations
├── frontend/             # Astro
├── tauri-app/            # Pupitre desktop app (Tauri v2)
├── helm/                 # Helm chart (all services)
├── infra/                # Kind config + manifests + course CRDs
├── docs/                 # Architecture, Course spec, Labs, SSO
└── examples/             # Sample Course CRD manifests
```

## Configuration

Each service is configured via a **ConfigMap mounted as a file** in the container. Environment variables coexist and override file values.

| Service | Mounted file | Key variables |
|---|---|---|
| **course-service** | `/etc/course-service/config.yaml` | `JWT_SECRET`, `DATABASE_URL`, `CHECKER_SERVICE_URL`, `GIT_TOKEN`, `LOG_LEVEL`, `LOG_FORMAT` |
| **user-service** | `/etc/user-service/config.yaml` | `JWT_SECRET`, `DATABASE_URL`, `OIDC_*`, `LOG_LEVEL`, `LOG_FORMAT` |
| **checker-service** | env only | `GITLAB_TOKEN`, `GITLAB_BASE_URL`, `LOG_LEVEL`, `LOG_FORMAT` |
| **frontend** | `/etc/frontend/config.env` | `COURSE_API`, `USER_API` |

Logging is controlled per service via two environment variables:

| Variable | Values | Default | Description |
|---|---|---|---|
| `LOG_LEVEL` | `debug` / `info` / `warn` / `error` | `info` | Log verbosity |
| `LOG_FORMAT` | `json` / `text` | `json` | Output format — `json` for log aggregators (ELK, DataDog), `text` for local dev |

```bash
# Human-readable logs for local development
LOG_LEVEL=debug LOG_FORMAT=text ./bin/user-service

# JSON logs for production / log aggregation
LOG_LEVEL=info LOG_FORMAT=json ./bin/user-service
```

## Git credentials — course-repo-secret

Courses with private git module repos require a K8s secret:

```bash
kubectl create secret generic course-repo-secret \
  --from-file=git-credentials.yaml=./git-credentials.yaml
```

`git-credentials.yaml` format:

```yaml
credentials:
  - url: "github.com/org/*"
    token: "ghp_xxx"
```

## Internal API

| Caller | Target | Endpoint | Description |
|---|---|---|---|
| Course Service | User Service | `GET /internal/enrollments/check` | Check enrollment |
| Course Service | User Service | `GET /internal/progress/viewed` | Get viewed lessons |
| Course Service | User Service | `POST /internal/progress/complete` | Mark lesson complete |
| Course Service | Checker Service | `POST /evaluate` | Evaluate lab submission |

## Known issue: Bitnami PostgreSQL

The Helm chart uses Bitnami PostgreSQL by default (`postgresql.enabled: true`), which may get stuck `Pending` on KinD due to `Insufficient ephemeral-storage`. If that happens, set `postgresql.enabled: false` in `infra/kind/kind-values.yaml` and run `make helm-install`.
