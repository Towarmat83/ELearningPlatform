# E-Learning Platform

## Architecture

The platform is split into two micro-services split from a single monolith:

```
┌──────────────────────┐     Internal HTTP      ┌──────────────────────┐
│    Course Service    │ ◄──────────────────────► │    User Service      │
│  (stateless, K8s)   │   /internal/enrollments  │   (PostgreSQL DB)    │
│                      │   /internal/progress    │                      │
│  Port 8082           │                          │  Port 8081            │
└──────────────────────┘                          └──────────────────────┘
```

### Course Service

Stateless service that owns course content. Source of truth is Kubernetes CRDs
(`elearning.example.com/v1`, kind `Course`). Watches the K8s API and maintains
an in-memory store. Calls User Service for enrollment checks and progress.

- **Source:** `course-service/`
- **API:** `course-service/openapi.yaml`
- **README:** `course-service/README.md`

### User Service

Database-backed service that owns auth, user profiles, OAuth, platform settings,
enrollments, and lesson progress. Exposes an internal REST API for Course Service.

- **Source:** `user-service/`
- **API:** `user-service/openapi.yaml`
- **README:** `user-service/README.md`

## Development

```sh
# Start PostgreSQL
docker run -d --name pg -e POSTGRES_USER=elearning -e POSTGRES_PASSWORD=elearning -e POSTGRES_DB=elearning -p 5432:5432 postgres:17

# Start User Service (DB required)
cd user-service && go run .

# Start Course Service (K8s CRD + User Service required)
cd course-service && go run .
```

## Internal API Contract

| Course Service → User Service | Description |
|---|---|
| `GET /internal/enrollments/check?user_id=&course_slug=` | Check enrollment |
| `GET /internal/progress/viewed?user_id=&course_slug=` | Get viewed lessons |
| `POST /internal/progress/complete` | Mark lesson complete |

## CRD Source of Truth

Courses are defined as Kubernetes Custom Resources:

```yaml
apiVersion: elearning.example.com/v1
kind: Course
metadata:
  name: kubernetes-basics
spec:
  title: "Kubernetes Basics"
  description: "Intro to K8s"
  hidden: false
  category: "kubernetes"
  difficulty: "beginner"
  modules:
    - name: "What is K8s"
      type: "text"
      src: "https://github.com/user/repo"
      ref: "main"
      path: "lessons/intro.md"
    - name: "Architecture Overview"
      type: "video"
      src: "/uploads/architecture.mp4"
```
