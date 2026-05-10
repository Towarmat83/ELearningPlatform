# Refactoring — GitOps Content Model

## Status: COMPLETED

This refactoring has been completed. The platform now uses the architecture described below.

## Objective

Split the monolith into two micro-services:
- **Course Service** (Go, stateless) — course content via K8s CRDs
- **User Service** (Go, PostgreSQL) — auth, enrollments, progress

## What changed

### Source of truth
Courses are now **Kubernetes CRDs** (`elearning.example.com/v1`, kind `Course`)
instead of filesystem YAML or database tables. The Course Service watches the K8s
API and maintains an in-memory store.

### Database tables removed
- `courses`, `labs`, `lessons`, `lab_submissions`, `lab_progress`, `lab_instances`

### New tables
- `enrollments` with `course_slug TEXT` (instead of `course_id UUID`)
- `lesson_progress(user_id, course_slug, lesson_slug)`

### Course Service (new)
- `internal/content/` — store + K8s CRD watcher + git content fetching
- `internal/handlers/` — chi router (courses, modules, lessons, labs)
- `internal/middleware/` — JWT auth validation
- `internal/config/` — env-based config
- `internal/metrics/` — Prometheus

### User Service (new)
- `internal/db/` — PostgreSQL connection + migration runner
- `internal/handlers/` — auth, OAuth, enrollments, progress, admin
- `internal/middleware/` — JWT auth create + validate
- `migrations/` — embedded SQL migrations

### Frontend
- Updated `hooks.server.ts` to proxy API routes to the correct services
- Updated API types and components for the new course/module model

### What was removed
- Rust API (`api-go/` with Axum, bollard, diesel)
- Interactive labs (Docker-in-browser terminal via WebSocket)
- Old lab system (submissions, instances, lab_progress tables)
- Local course filesystem loader (`COURSES_DIR`)

## CRD format

```yaml
apiVersion: elearning.example.com/v1
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

## Verification

```bash
# Deploy
make dev
make port-forward

# Test
curl http://localhost:18082/api/courses
curl http://localhost:18081/health

# Enroll as admin
# Open http://localhost:3000 → admin@elearning.local / Admin@1234
```
