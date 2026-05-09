# Course Service

Stateless micro-service that serves course/module content for the e-learning platform.

## Architecture

- **Stateless** — no database. All course data comes from Kubernetes CRDs (`elearning.example.com/v1`, kind `Course`) via an in-cluster watcher.
- **Source of truth** — courses are defined as Kubernetes custom resources. The service watches the K8s API and populates an in-memory store.
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
| GET | `/api/courses/{slug}/modules/{index}` | JWT | Get module with content |
| GET | `/api/courses/{slug}/lessons` | JWT | List lessons with viewed status |
| GET | `/api/courses/{slug}/lessons/{lesson_slug}` | JWT | Get lesson content |
| POST | `/api/courses/{slug}/lessons/{lesson_slug}/complete` | JWT | Mark lesson complete |
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

## How to Run

### Locally

```sh
go run main.go
```

Requires a K8s cluster with the `elearning.example.com/v1` Course CRD installed and at least one Course resource.

### Docker

```sh
docker build -t course-service .
docker run -p 8082:8082 -e KUBECONFIG=/app/kubeconfig -v /path/to/kubeconfig:/app/kubeconfig course-service
```

### Kubernetes

Deploy as a standard Deployment. The service will use in-cluster config to watch Course CRDs.

## Dependencies

- **Kubernetes cluster** with `elearning.example.com/v1` Course CRD installed
- **User Service** running and accessible via `USER_SERVICE_URL`
