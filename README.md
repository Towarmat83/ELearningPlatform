# Pupitre — E-Learning Platform

Micro-services e-learning platform with Kubernetes CRD-based course definitions, interactive lab validation, and sequential learning paths with skill tracking.

## Table of contents

- [Architecture](#architecture)
- [Project structure](#project-structure)
- [Services](#services)
- [Frontend](#frontend)
- [CRDs — source of truth](#crds--source-of-truth)
- [Learning paths & skills](#learning-paths--skills)
- [Interactive labs](#interactive-labs)
- [Pupitre — desktop app](#pupitre--desktop-app)
- [Configuration](#configuration)
- [Internal API](#internal-api)
- [CI & Makefile](#ci--makefile)

---

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│  Clients                                                        │
│  Browser ──────────────────────────────────────────────────┐   │
│  Pupitre (Tauri desktop) ──────────────────────────────────┤   │
└────────────────────────────────────────────────────────────┼───┘
                                                             │
                        ┌────────────────────────────────────▼──┐
                        │  Frontend — Astro SSR  :3000           │
                        │  API proxy · markdown renderer         │
                        └──────────┬────────────────────────────┘
                                   │  HTTP
              ┌────────────────────┼──────────────────┐
              │                    │                   │
    ┌─────────▼──────┐  ┌──────────▼──────┐  ┌───────▼────────┐
    │  User Service  │  │ Course Service  │  │Checker Service │
    │  :8081         │  │ :8082           │  │ :8083          │
    │                │  │                 │  │                │
    │ Auth (local    │  │ Courses · Labs  │  │ GitLab fetch   │
    │ + OAuth/OIDC)  │  │ Quiz · Skills   │  │ OPA/Rego eval  │
    │ Enrollments    │  │ Paths · Modules │  │ Step checker   │
    │ Progress       │  │ Lab results     │  │                │
    │ Admin · Groups │  │ K8s CRD watcher │  │                │
    └────────┬───────┘  └──────┬──────────┘  └───────────────┘
             │                 │                      │
             │                 ├──────────────────────┘
             │                 │    Internal calls
    ┌────────▼─────────────────▼────────────────────────┐
    │  PostgreSQL                                        │
    │  user-service DB  ·  course-service DB             │
    └────────────────────────────────────────────────────┘
             
    ┌──────────────────┐   ┌────────────────┐   ┌──────────────┐
    │  Kubernetes CRDs │   │   Git repos    │   │   GitLab     │
    │  Kind:Course     │   │  Module content│   │  Student MRs │
    │  Kind:Path       │   │  Lab assets    │   │  Pipelines   │
    │  Kind:Pattern    │   │  check.rego    │   │  Commits     │
    └──────────────────┘   └────────────────┘   └──────────────┘
```

---

## Project structure

```
pupitre/
│
├── course-service/            # Go service — port 8082
│   ├── main.go
│   ├── Dockerfile
│   ├── migrations/            # SQL migrations (embedded at build time)
│   │   ├── 001_lab_checks.sql
│   │   └── 002_lab_checks_verified.sql
│   ├── internal/
│   │   ├── config/            # Env / file config loading
│   │   ├── content/           # Core domain logic
│   │   │   ├── k8s.go         # K8s CRD watcher (Course, Path, Pattern)
│   │   │   ├── loader.go      # Module loader (git fetch + CRD merge)
│   │   │   ├── git.go         # Git clone / fetch module content
│   │   │   ├── credentials.go # Git credential store
│   │   │   ├── types.go       # Course, Module, Path domain types
│   │   │   ├── quiz_types.go  # Quiz question types + scoring
│   │   │   ├── scoring.go     # Quiz answer evaluation
│   │   │   ├── cooldown.go    # Quiz question cooldown logic
│   │   │   ├── crypto.go      # Answer hashing
│   │   │   ├── replicate.go   # Cross-cluster replication helpers
│   │   │   └── paths.go       # Path aggregation (courses + skills)
│   │   ├── db/                # PostgreSQL connection + migrations runner
│   │   ├── handlers/          # HTTP route handlers
│   │   │   ├── router.go      # Route registration
│   │   │   ├── state.go       # Handler state (DB, store, config)
│   │   │   ├── courses.go     # GET /api/courses, /api/courses/:slug
│   │   │   ├── modules.go     # GET /api/courses/:slug/modules[/:idx]
│   │   │   ├── lessons.go     # POST /complete, progress tracking
│   │   │   ├── check.go       # POST /check — lab validation proxy
│   │   │   ├── lab_results.go # GET /admin/labs — all lab attempts
│   │   │   ├── paths.go       # GET /api/paths/:slug
│   │   │   ├── skills.go      # GET /api/skills/:slug/modules
│   │   │   ├── courses_crd.go # Admin CRD CRUD
│   │   │   └── health.go      # GET /healthz
│   │   ├── middleware/        # JWT auth middleware
│   │   └── metrics/           # Prometheus metrics
│   ├── api/v1/                # Generated K8s CRD Go types
│   └── config/crd/            # CRD YAML definitions (Course, Path, Pattern)
│
├── user-service/              # Go service — port 8081
│   ├── Dockerfile
│   ├── migrations/            # SQL migrations (embedded at build time)
│   │   ├── 001_init.sql               # users, sessions tables
│   │   ├── 002_groups.sql             # groups
│   │   ├── 002_course_settings.sql    # per-course settings
│   │   ├── 003_oidc_browser_url.sql   # OIDC config
│   │   ├── 003_cleanup_unused.sql
│   │   ├── 004_module_progress.sql    # module_progress table
│   │   ├── 004_default_group.sql
│   │   ├── 005_module_progress_slug.sql
│   │   ├── 005_group_enrollments.sql
│   │   ├── 006_markdown_patterns.sql
│   │   └── 007_path_enrollments.sql
│   └── internal/
│       ├── config/
│       ├── db/
│       ├── middleware/
│       └── handlers/
│           ├── router.go      # Route registration
│           ├── state.go       # Handler state
│           ├── auth.go        # POST /login, /register, /logout
│           ├── oauth.go       # OAuth2 PKCE flow
│           ├── oidc.go        # OIDC (Keycloak / GitHub)
│           ├── ldap.go        # LDAP auth
│           ├── admin.go       # Admin user management
│           ├── groups.go      # Groups & enrollments
│           ├── enrollments.go # Path enrollment
│           ├── progress.go    # lesson_progress, module_progress
│           ├── paths.go       # GET /api/my/paths — enrolled paths + status
│           ├── skills.go      # GET /api/my/skills/:slug — skill modules + status
│           ├── patterns.go    # Markdown pattern admin
│           ├── pattern_watcher.go
│           ├── settings.go    # Platform settings
│           └── internal.go    # Internal endpoints (called by course-service)
│
├── checker-service/           # Go service — port 8083
│   ├── Dockerfile
│   └── internal/
│       ├── config/
│       ├── handlers/          # POST /evaluate
│       └── checker/
│           ├── fetch.go       # Fetch GitLab state (MRs, pipelines, files, commits)
│           ├── eval.go        # OPA/Rego policy evaluation
│           ├── step.go        # Per-step lab check logic
│           └── types.go       # CheckRequest / CheckResponse types
│
├── frontend/                  # Astro SSR — port 3000
│   ├── Dockerfile
│   ├── astro.config.mjs
│   └── src/
│       ├── layouts/
│       │   ├── App.astro      # Main layout (navbar, auth guard)
│       │   ├── Admin.astro    # Admin layout
│       │   └── Base.astro     # Bare HTML shell
│       ├── lib/
│       │   ├── api.ts         # apiFetch helper (token injection, error handling)
│       │   └── local-check.ts # Tauri local_check command wrapper
│       ├── styles/            # Global CSS
│       └── pages/
│           ├── index.astro            # Home / redirect
│           ├── login.astro            # Login (local + OAuth)
│           ├── register.astro         # Registration
│           ├── dashboard.astro        # User dashboard
│           ├── profile.astro          # User profile
│           ├── my-courses.astro       # All enrolled courses
│           ├── my-paths.astro         # Learning paths (snake view)
│           ├── skills/
│           │   └── [slug].astro       # Skill detail (module snake)
│           ├── courses/
│           │   ├── index.astro        # Course catalogue
│           │   └── [slug]/
│           │       ├── index.astro    # Course overview + module list
│           │       ├── lessons/
│           │       │   └── [index].astro   # Lesson / lab page
│           │       └── quiz/
│           │           └── [index].astro   # Quiz page
│           ├── admin/
│           │   ├── index.astro        # Admin home
│           │   ├── [...slug].astro    # Dynamic admin route
│           │   ├── courses/           # Course management
│           │   ├── users/             # User management
│           │   ├── groups/            # Group management
│           │   ├── paths/             # Learning path management
│           │   ├── labs/              # Lab results review
│           │   ├── leaderboard/       # Leaderboard
│           │   ├── monitoring/        # Metrics / Prometheus
│           │   ├── patterns/          # Markdown pattern admin
│           │   └── settings/          # Platform settings
│           └── api/                   # Astro API routes (proxy to backend)
│               └── auth/
│                   └── callback.astro # OAuth2 callback handler
│
├── helm/                      # Helm chart (git submodule → genesary/pupitre-helm)
│   ├── Chart.yaml
│   ├── values.yaml
│   ├── crds/                  # CRD manifests installed by Helm
│   │   ├── elearning.pupitre.io_courses.yaml
│   │   ├── elearning.pupitre.io_paths.yaml
│   │   └── elearning.pupitre.io_markdownpatterns.yaml
│   └── templates/
│       ├── course-service/
│       ├── user-service/
│       ├── checker-service/
│       ├── frontend/
│       ├── ingress.yaml
│       └── secret.yaml
│
├── pupitre-desktop/           # Tauri v2 desktop app (git submodule → genesary/pupitre-desktop)
│   ├── package.json
│   └── src-tauri/
│       ├── Cargo.toml
│       ├── tauri.conf.json
│       └── src/
│           ├── main.rs        # Entry point → lib::run()
│           └── lib.rs         # Tauri commands (local_check, podman wrappers)
│
├── infra/
│   ├── kind/                  # KinD cluster config
│   ├── manifests/             # Raw K8s manifests
│   ├── courses/               # Course CRD instances (applied via make apply-courses)
│   └── dev/                   # Local dev setup scripts (GitLab, Keycloak, CNPG)
│
├── examples/
│   ├── courses/               # Sample Course CRD manifests per course
│   ├── quizzes/               # Sample quiz YAML files
│   └── k8s/                   # Sample K8s deployment manifests
│
├── e2e/                       # End-to-end Go tests
│   ├── main_test.go
│   ├── course_test.go
│   ├── user_test.go
│   ├── checker_test.go
│   └── helpers_test.go
│
├── docs/
│   ├── ARCHITECTURE.md        # Detailed architecture notes
│   ├── Course.md              # CRD Course spec reference
│   ├── Module.md              # Module types reference
│   ├── interactive-labs.md    # Lab system deep-dive
│   └── SSO.md                 # SSO / OIDC setup guide
│
├── .github/workflows/
│   ├── ci-golang.yaml         # Go lint + tests (all 3 services)
│   ├── ci-containers.yaml     # Docker build + push
│   ├── ci-e2e.yaml            # E2E tests on KinD
│   └── release.yaml           # Release (tag → chart bump)
│
├── Makefile                   # Dev commands (see below)
├── CONTRIBUTING.md
├── AGENTS.md                  # AI agent guidelines
└── cliff.toml                 # Changelog generation config
```

---

## Services

### Course Service `:8082`

Owns all course content. Watches K8s CRDs for `Course`, `Path`, and `MarkdownPattern` objects. On each CRD event, fetches module content (markdown, quiz YAML, lab assets) from Git. Exposes the course catalogue, module content, quiz engine, lab check proxy, and progress endpoints.

**Key tables:** `lab_checks`

**Key dependencies:** Kubernetes API (CRD watcher), Git (module content), User Service (internal), Checker Service (lab evaluation)

### User Service `:8081`

Owns all user data, authentication, and progress tracking. Supports local login, OAuth2 (PKCE), OIDC (Keycloak, GitHub), and LDAP. Manages enrollments, groups, and per-user module/lesson progress.

**Key tables:** `users`, `sessions`, `groups`, `group_members`, `module_progress`, `lesson_progress`, `path_enrollments`, `markdown_patterns`

**Key dependencies:** PostgreSQL, external OIDC provider (optional)

### Checker Service `:8083`

Stateless. Receives a `CheckRequest` from course-service, fetches live GitLab state (MR status, pipeline results, commits, files), evaluates an OPA/Rego policy, and returns `{allow, violations}`. Also supports per-step local checks.

**Key dependencies:** GitLab API, Git (Rego files), OPA

---

## Frontend

Astro SSR app, acts as API gateway for the browser. All `/api/*` calls are proxied through Astro to the backend services (avoids CORS, centralises auth token handling).

| Page | Route | Description |
|---|---|---|
| Home | `/` | Redirect based on auth state |
| Login | `/login` | Local + OAuth2/OIDC login |
| Dashboard | `/dashboard` | User overview |
| Catalogue | `/courses` | All available courses |
| Course | `/courses/:slug` | Course overview, module list with sequential locking |
| Lesson | `/courses/:slug/lessons/:idx` | Lesson content, lab steps, inline quiz |
| Quiz | `/courses/:slug/quiz/:idx` | Standalone quiz with cooldowns |
| My Courses | `/my-courses` | Enrolled courses |
| My Paths | `/my-paths` | Learning paths — snake view, per-node status |
| Skill | `/skills/:slug` | Skill detail — ordered module snake |
| Admin | `/admin/*` | Admin panel (users, groups, courses, paths, labs, monitoring) |

### Sequential locking

- **Course modules**: module N is locked until module N-1 is completed (enforced server-side in `ListModules` + client-side redirect guard).
- **Path courses**: course N is locked until course N-1 is completed.
- **Skill modules**: same `prevCompleted` pattern within each skill.
- **Path skills**: skill N is locked until skill N-1 is completed (all its quiz/lab modules done).

---

## CRDs — source of truth

Courses are defined as Kubernetes Custom Resources (`elearning.pupitre.io/v1`). Three CRD kinds:

### `Kind: Course`

Defines a course and its ordered module list.

```yaml
apiVersion: elearning.pupitre.io/v1
kind: Course
metadata:
  name: docker-fundamentals
spec:
  title: "Les fondamentaux de Docker"
  description: "..."
  gitRepo: "https://github.com/org/course-docker.git"
  modules:
    - name: "Images et conteneurs"
      type: lesson       # lesson | quiz | lab
      path: modules/01-images
    - name: "Quiz Docker"
      type: quiz
      inline: false      # true = embedded in preceding lesson
    - name: "Lab Harbor"
      type: lab
      checkProvider: gitlab   # gitlab | local
      path: modules/lab-harbor
```

Module types: `lesson`, `quiz`, `lab`. See [`docs/Course.md`](docs/Course.md) for the full spec.

### `Kind: Path`

Defines a learning path — an ordered list of courses or skills.

```yaml
apiVersion: elearning.pupitre.io/v1
kind: Path
metadata:
  name: devops-path
spec:
  title: "Parcours DevOps"
  kind: course     # course | skill
  courses:
    - linux-intro
    - docker-fundamentals
    - kubernetes-basics
```

- `kind: course` → nodes are course slugs, sequential unlocking based on course completion.
- `kind: skill` → nodes are skill slugs, sequential unlocking based on skill completion (all assessable modules done).

### `Kind: MarkdownPattern`

Named regex → replacement pairs applied globally to all markdown content at render time.

---

## Learning paths & skills

**Skill** = a cross-cutting topic (e.g. `reseaux`, `securite-conteneurs`) that groups modules from multiple courses.

Each module in a `Course` CRD can declare one or more `skills: [...]` tags. The platform aggregates all modules tagged with a given skill into an ordered skill view.

**Completion rules:**
- A quiz module is completed when `module_progress.passed = true`.
- A lab module is completed when a `lesson_progress` row with the module slug exists (written by `POST /complete` after all steps pass).
- A lesson/text module is completed when viewed (any `lesson_progress` row for its slug).
- A skill is completed when **all** its quiz and lab modules are done.

**Progress is not path-scoped**: completing a module in one path/skill automatically counts in all other paths/skills that reference the same module.

---

## Interactive labs

Lab modules (`type: lab`) display a markdown assignment inline and let students validate their work.

### GitLab labs (`checkProvider: gitlab`)

Each lab requires two files in the git repo alongside `content.md`:

```
modules/lab1/
  ├── content.md    # Assignment markdown
  ├── check.yaml    # Provider config, project template, files to verify
  └── check.rego    # OPA/Rego validation policy
```

Validation flow:

```
Student clicks "Vérifier mon travail"
  → course-service reads check.yaml + check.rego from git
  → POST checker-service /evaluate
  → checker fetches live GitLab state (MRs, commits, pipelines, files)
  → evaluates Rego → {allow, violations}
  → stored in lab_checks table
  → result displayed to student
```

Labs can also have **per-step validation** — each step has its own check, and all steps must pass before the lab is marked complete (which triggers `POST /complete` and redirects to the next module).

Instructors review all attempts at `/admin/labs`.

### Local labs (`checkProvider: local`)

For labs that run on the student's local machine (e.g. `podman pull`, `podman run`). Requires the Pupitre desktop app.

```
Student clicks "Vérifier" (inside Pupitre)
  → frontend detects window.__TAURI_INTERNALS__
  → invoke("local_check", { checkType, params })
  → Rust runs podman commands locally
  → {allow, violations} returned to frontend
```

See [`docs/interactive-labs.md`](docs/interactive-labs.md) for full documentation.

---

## Pupitre — desktop app

A **Tauri v2** desktop app that wraps the frontend in a WebView and adds local machine access via Rust commands. Lives in a separate git submodule: [`pupitre-desktop`](https://github.com/genesary/pupitre-desktop).

```
pupitre-desktop/
├── package.json
└── src-tauri/
    ├── Cargo.toml
    ├── tauri.conf.json
    ├── capabilities/      # Tauri permission system
    ├── icons/
    └── src/
        ├── main.rs        # Entry point
        └── lib.rs         # Tauri commands: local_check, podman wrappers
```

Build instructions: see the [`pupitre-desktop`](https://github.com/genesary/pupitre-desktop) repo.

---

## Quick start

```bash
# Full local cluster (KinD + Helm + courses)
make dev

# Rebuild a single service and reload in KinD
make rebuild-course    # or rebuild-user / rebuild-frontend / rebuild-checker

# Stream all service logs
make logs

# Tear everything down
make clean
```

### Makefile targets

| Target | Description |
|---|---|
| `dev` | Full setup: create KinD cluster, build images, load, Helm install, apply courses |
| `kind-create` / `kind-delete` | KinD cluster lifecycle |
| `docker-build` | Build all four Docker images |
| `kind-load` | Load images into KinD (no registry needed) |
| `helm-install` / `helm-delete` | Deploy / remove via Helm |
| `apply-courses` | Apply Course CRD manifests from `infra/courses/` |
| `rebuild-<service>` | Build + load a single service |
| `port-forward` | Forward service ports to localhost |
| `openapi-gen` | Regenerate OpenAPI specs |
| `create-git-secret` | Create K8s secret for private git repos |

---

## Configuration

Each service reads config from a **mounted file** (ConfigMap) or environment variables. Env vars override file values.

| Service | Config file | Key variables |
|---|---|---|
| **course-service** | `/etc/course-service/config.yaml` | `JWT_SECRET`, `DATABASE_URL`, `CHECKER_SERVICE_URL`, `GIT_TOKEN`, `LOG_LEVEL`, `LOG_FORMAT` |
| **user-service** | `/etc/user-service/config.yaml` | `JWT_SECRET`, `DATABASE_URL`, `OIDC_*`, `LOG_LEVEL`, `LOG_FORMAT` |
| **checker-service** | env only | `GITLAB_TOKEN`, `GITLAB_BASE_URL`, `LOG_LEVEL`, `LOG_FORMAT` |
| **frontend** | `/etc/frontend/config.env` | `COURSE_API`, `USER_API` |

### Log level / format

All services support runtime log tuning via two env vars:

| Variable | Values | Default |
|---|---|---|
| `LOG_LEVEL` | `debug` / `info` / `warn` / `error` | `info` |
| `LOG_FORMAT` | `json` / `text` | `json` |

```bash
# Human-readable for local dev
LOG_LEVEL=debug LOG_FORMAT=text ./bin/user-service

# JSON for production / log aggregators
LOG_LEVEL=info LOG_FORMAT=json ./bin/user-service
```

### Git credentials (private course repos)

```bash
kubectl create secret generic course-repo-secret \
  --from-file=git-credentials.yaml=./git-credentials.yaml
```

`git-credentials.yaml`:
```yaml
credentials:
  - url: "github.com/org/*"
    token: "ghp_xxx"
```

---

## Internal API

Calls between services use shared JWT auth and a dedicated `/internal/*` prefix on user-service.

| Caller | Target | Endpoint | Purpose |
|---|---|---|---|
| course-service | user-service | `GET /internal/enrollments/check` | Verify student is enrolled in a course |
| course-service | user-service | `GET /internal/progress/viewed` | Fetch viewed lessons for sequential lock |
| course-service | user-service | `POST /internal/progress/complete` | Mark a lesson/lab as complete |
| course-service | checker-service | `POST /evaluate` | Evaluate a lab submission |

---

## CI & Makefile

| Workflow | Trigger | Steps |
|---|---|---|
| `ci-golang.yaml` | PR / push | golangci-lint + `go test` on all 3 services |
| `ci-containers.yaml` | push to `main` | Docker build + push to registry |
| `ci-e2e.yaml` | PR / push | Spin KinD, deploy via Helm, run `e2e/` Go tests |
| `release.yaml` | Git tag | Helm chart version bump + GitHub release |

---

## Known issue: Bitnami PostgreSQL on KinD

The Helm chart uses Bitnami PostgreSQL (`postgresql.enabled: true`) which can get stuck `Pending` on KinD due to `Insufficient ephemeral-storage`. Fix: set `postgresql.enabled: false` in `infra/kind/kind-values.yaml` and use an external PostgreSQL or CloudNativePG, then run `make helm-install`.
