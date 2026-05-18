# E-Learning Platform

Micro-services platform for online courses with Kubernetes CRD-based course definitions.

## Architecture

```
┌──────────┐     ┌──────────────┐     ┌──────────────┐
│ Frontend  │────▶│ Course API   │◀───▶│ K8s CRD      │
│ :3000     │     │ :8082        │     │ (courses)    │
└────┬──────┘     └──────┬───────┘     └──────────────┘
     │                   │
     │            ┌──────▼───────┐     ┌──────────────┐
     └───────────▶│ User API     │◀───▶│ PostgreSQL   │
                  │ :8081        │     │ :5432        │
                  └──────────────┘     └──────────────┘
```

| Service | Role | Tech |
|---------|------|------|
| **Course Service** | Course content, media serving, K8s CRD watcher, quiz scoring & cooldown | Go + chi + client-go |
| **User Service** | Auth, enrollments, progress, admin | Go + chi + pgx |
| **Frontend** | SvelteKit SPA, server-side API proxy | SvelteKit |

## Source of truth

Courses are Kubernetes CRDs (`elearning.example.com/v1`, kind `Course`).

```yaml
apiVersion: elearning.example.com/v1
kind: Course
metadata:
  name: kubernetes-basics
spec:
  title: "Kubernetes Basics"
  description: "Learn the fundamentals of Kubernetes"
  hidden: false
  category: "kubernetes"
  difficulty: "beginner"
  modules:
    - name: "What is Kubernetes"
      type: "text"
      src: "https://github.com/user/repo"
      ref: "main"
      path: "lessons/intro.md"
    - name: "Architecture Overview"
      type: "video"
      src: "/uploads/architecture.mp4"
    - name: "Kubernetes Basics Quiz"
      type: "quiz"
      passing_score: 80
      max_attempts_per_question: 3
      lock_on_max_attempts: true
      cooldown:
        strategy: "exponential"
        base_seconds: 30
        multiplier: 2.0
        max_seconds: 600
      questions:
        - id: "q1"
          type: "single"
          points: 1
          question: "What is a Pod?"
          answers:
            - id: "a"
              text: "Smallest deployable unit"
              correct: true
            - id: "b"
              text: "A physical node"
              correct: false
```

Module types: `text` (markdown from git), `video` / `image` (server-hosted URL), `quiz` (inline or git-fetched questions).

## Quick start

```bash
make dev
```

See `CONTRIB.md` for the full step-by-step guide.

## Known issue: Bitnami PostgreSQL

The Helm chart uses Bitnami PostgreSQL by default (`postgresql.enabled: true`), which may get stuck `Pending` on KinD due to `Insufficient ephemeral-storage`. If that happens, set `postgresql.enabled: false` in `deploy/kind-values.yaml` and run `make helm-install` — the Makefile auto-deploys a standalone PostgreSQL (`deploy/postgresql.yaml`) instead.

## Project structure

```
./
├── course-service/       # Go service (port 8082)
│   ├── internal/
│   │   ├── content/      # Store, K8s watcher, git fetch, quiz types & scoring
│   │   ├── handlers/     # HTTP routes
│   │   ├── middleware/   # JWT auth
│   │   ├── config/      # Env config
│   │   └── metrics/     # Prometheus
│   └── examples/        # Sample k8s manifests
├── user-service/         # Go service (port 8081)
│   ├── internal/
│   │   ├── db/          # PG + migrations
│   │   ├── handlers/    # Auth, OAuth, admin, progress
│   │   ├── middleware/  # JWT
│   │   └── config/      # Env config
│   └── migrations/      # SQL migrations (embed)
├── frontend/            # SvelteKit
├── helm/                # Helm chart
└── courses/             # Course CRD manifests (examples)
```

## Configuration

Chaque service est configuré via une **ConfigMap montée en fichier** dans le conteneur. Les variables d'environnement coexistent et surchargent les valeurs du fichier.

| Service | Fichier monté |
|---|---|
| **course-service** | `/etc/course-service/config.yaml` |
| **user-service** | `/etc/user-service/config.yaml` |
| **frontend** | `/etc/frontend/config.env` (sourcé par l'entrypoint) |

Voir `docs/ARCHITECTURE.md` (section "Configuration des services") pour les détails.

## Git credentials — course-repo-secret

Les cours avec modules git privés nécessitent un secret K8s :

```bash
kubectl create secret generic course-repo-secret \
  --from-file=git-credentials.yaml=./git-credentials.yaml
```

Voir `course-service/examples/course-secret.yaml` pour le format.

## Internal API

| Course Service → User Service | Description |
|---|---|
| `GET /internal/enrollments/check?user_id=&course_slug=` | Check enrollment |
| `GET /internal/progress/viewed?user_id=&course_slug=` | Get viewed lessons |
| `POST /internal/progress/complete` | Mark lesson complete |
