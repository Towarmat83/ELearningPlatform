# User Service

Database-backed micro-service for the e-learning platform. Handles all user-facing data: authentication, profiles, enrollments, lesson progress, and platform settings.

## Architecture

- **PostgreSQL** for all persistence (users, enrollments, progress, settings)
- **JWT** bearer tokens for authentication
- **OAuth 2.0** (GitLab, GitHub) for SSO login
- **Internal REST API** for the Course Service (enrollment checks, progress queries)
- **Prometheus** metrics at `/metrics`

### API endpoints

| Group | Method | Path | Auth |
|---|---|---|---|
| **Public** | GET | `/health` | — |
| | GET | `/metrics` | — |
| | GET | `/api/settings/public` | — |
| | POST | `/api/auth/register` | — |
| | POST | `/api/auth/login` | — |
| | GET | `/api/auth/oauth/providers` | — |
| | GET | `/api/auth/oauth/{provider}/authorize` | — |
| | POST | `/api/auth/oauth/callback` | — |
| **Authenticated** | GET | `/api/auth/me` | JWT |
| | PUT | `/api/auth/profile` | JWT |
| | PUT | `/api/auth/password` | JWT |
| | POST | `/api/courses/{slug}/enroll` | JWT |
| | DELETE | `/api/courses/{slug}/unenroll` | JWT |
| | POST | `/api/courses/{slug}/lessons/{lesson_slug}/complete` | JWT |
| | GET | `/api/my/courses` | JWT |
| **Admin** | GET | `/api/admin/settings` | Admin |
| | PUT | `/api/admin/settings` | Admin |
| | GET | `/api/admin/stats` | Admin |
| | GET | `/api/admin/users` | Admin |
| | GET | `/api/admin/users/search` | Admin |
| | GET | `/api/admin/users/{user_id}` | Admin |
| | PUT | `/api/admin/users/{user_id}` | Admin |
| | DELETE | `/api/admin/users/{user_id}` | Admin |
| | GET | `/api/admin/courses/{slug}/enrollments` | Admin |
| | POST | `/api/admin/courses/{slug}/enrollments` | Admin |
| | DELETE | `/api/admin/courses/{slug}/enrollments/{user_id}` | Admin |
| | POST | `/api/admin/sync-progress` | Admin |
| **Internal** | GET | `/internal/enrollments/check` | Network policy |
| | GET | `/internal/progress/viewed` | Network policy |
| | POST | `/internal/progress/complete` | Network policy |

## Configuration

Configured via environment variables:

| Variable | Default | Description |
|---|---|---|
| `DATABASE_URL` | `postgres://elearning:elearning@localhost:5432/elearning` | PostgreSQL connection string |
| `JWT_SECRET` | `change-me-in-production-use-a-long-random-string` | HMAC key for JWT signing |
| `JWT_EXPIRY_HOURS` | `24` | Token lifetime in hours |
| `PORT` | `8081` | HTTP listen port |
| `CORS_ORIGINS` | `http://localhost:3000,http://localhost:5173` | Allowed CORS origins |
| `GITLAB_CLIENT_ID` | — | GitLab OAuth app ID |
| `GITLAB_CLIENT_SECRET` | — | GitLab OAuth app secret |
| `GITLAB_URL` | `https://gitlab.com` | GitLab instance URL |
| `GITHUB_CLIENT_ID` | — | GitHub OAuth app ID |
| `GITHUB_CLIENT_SECRET` | — | GitHub OAuth app secret |
| `OAUTH_REDIRECT_BASE` | `http://localhost:3000` | Frontend base URL for OAuth redirect |
| `COURSE_SERVICE_URL` | `http://course-service:8080` | Internal Course Service URL |

## Database

Uses PostgreSQL with automatic migrations on startup. Migrations are embedded SQL files in `migrations/`.

### Schema

- `users` — user accounts (local + SSO)
- `platform_settings` — key/value app configuration
- `enrollments` — course enrollment records
- `lesson_progress` — per-user per-lesson tracking
- `course_settings` — publishing and auto-enroll overrides
- `git_repos` — user-connected git repository tokens

## Running

### Locally

```sh
# Start PostgreSQL (e.g. via Docker)
docker run -d --name pg \
  -e POSTGRES_USER=elearning \
  -e POSTGRES_PASSWORD=elearning \
  -e POSTGRES_DB=elearning \
  -p 5432:5432 postgres:17

# Run the service
go run .
```

### Docker

```sh
docker build -t user-service .
docker run -p 8081:8081 \
  -e DATABASE_URL=postgres://elearning:elearning@host.docker.internal:5432/elearning \
  user-service
```

## Development Workflow (KinD)

### Quick rebuild after code changes

```sh
# From project root — build, load, restart in one command
make rebuild-user

# Or manually:
docker build -t localhost/elearning-user-service:latest user-service/
kind load docker-image localhost/elearning-user-service:latest --name elearning
kubectl rollout restart deploy/elearning-user-service

# Check logs
make logs
```

### Local testing (with Docker PostgreSQL)

```sh
# Start PostgreSQL
docker run -d --name pg \
  -e POSTGRES_USER=elearning \
  -e POSTGRES_PASSWORD=elearning \
  -e POSTGRES_DB=elearning \
  -p 5432:5432 postgres:17

# Run the service
cd user-service && go run .
```

## Dependencies

- Go 1.26+
- PostgreSQL 15+
