# e-learning platform

## MANDATORY — NEVER COMMIT

You are strictly forbidden from running `git commit`, `git add`, `git push`, or any other git mutation commands. Only the user may commit. If you are asked to commit or you think a commit is needed, refuse and tell the user to do it themselves.

## Domain model (source of truth: `docs/`)

- **Course** — has a title, description, hidden (boolean publication state), category, difficulty, and a list of modules. Stored in PostgreSQL and managed through the course-service admin API. Modules carry their markdown inline (`content`), or reference external content by type (`video`, `text`, `image`) and a git repo (`src`, `ref`, `path`).
- **Module** — a course element. Video/image: server-hosted URL. Text: inline markdown stored on the module, or a markdown file from a git repo. Inline is the default authoring path; git is for content that already lives in a repo.
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
| `internal/markdown/` | Course ⇄ single markdown document: frontmatter, heading splitting, module directives |
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

POST   /api/admin/courses/import               # create/replace/append from markdown
POST   /api/admin/courses/import/preview       # same, but stores nothing
GET    /api/admin/courses/{slug}/export/markdown
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
      content: |
        Kubernetes is a container orchestrator.
    - name: "From a repo"
      type: "text"
      src: "https://github.com/user/repo"
      ref: "main"
      path: "lessons/intro.md"
```

A module's body is `content` (markdown stored on the module) **or** the
`src`/`ref`/`path` triple (a file in a git repo). Git wins when both are set,
so writers clear one when they set the other.

## Markdown import/export

A whole course also round-trips through one markdown document — the path the
admin UI leads with, since it needs no repository:

```markdown
---
slug: kubernetes-basics
title: Kubernetes Basics
category: kubernetes
split: h2
---

## What is Kubernetes

A container orchestrator.

## Knowledge check

<!--pupitre
type: quiz
passingScore: 80
questions: [...]
-->
```

- Frontmatter is the course definition, keyed exactly like the admin API's `spec`.
- Modules are cut at the heading level named by `split` (`none`, `h1`…`h6`); a
  request may override it.
- A `<!--pupitre …-->` comment under a heading carries whatever markdown cannot
  express (type, git source, quiz questions, skills, prerequisites). No renderer
  shows it, so an exported course stays readable as plain markdown.
- Export picks a heading level no module body already uses and records it in
  `split`, so `export → import` is lossless.
- Admin routes accept 10 MB bodies (`maxAdminRequestBodyBytes`); everything else
  stays at 1 MB.

## Build notes

- Go 1.26
- Course Service: chi, JWT, Prometheus, GORM/pgx, go-git
- User Service: chi, pgx, JWT, Prometheus, godotenv, crypto
- `DATABASE_URL` is required by both services; course-service refuses to start without it
- `SEED_DEV_COURSES=true` seeds the embedded demo catalogue (`course-service/internal/db/seed/courses/`); `overwrite` replaces it. Unset in production.
