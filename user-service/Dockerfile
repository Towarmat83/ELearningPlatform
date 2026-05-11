# Stage 1: Build
FROM golang:1.26-alpine AS builder

RUN apk add --no-cache ca-certificates git

WORKDIR /app

# Cache module downloads
COPY go.mod go.sum ./
RUN go mod download

# Build binary
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o user-service .

# Stage 2: Runtime
FROM alpine:3.21

RUN apk add --no-cache ca-certificates curl

WORKDIR /app

COPY --from=builder /app/user-service /app/user-service

# ── All configuration via env vars (ConfigMap friendly) ──────────────────────

ENV PORT=8081 \
    CONFIG_PATH="" \
    DATABASE_URL="postgres://elearning:elearning@localhost:5432/elearning" \
    JWT_SECRET="change-me-in-production-use-a-long-random-string" \
    JWT_EXPIRY_HOURS=24 \
    CORS_ORIGINS="http://localhost:3000,http://localhost:5173" \
    GITLAB_CLIENT_ID="" \
    GITLAB_CLIENT_SECRET="" \
    GITLAB_URL="https://gitlab.com" \
    GITHUB_CLIENT_ID="" \
    GITHUB_CLIENT_SECRET="" \
    OAUTH_REDIRECT_BASE="http://localhost:3000" \
    COURSE_SERVICE_URL="http://elearning-course-service:8082"

EXPOSE 8081

HEALTHCHECK --interval=30s --timeout=10s --start-period=10s --retries=3 \
    CMD curl -f http://localhost:8081/health || exit 1

ENTRYPOINT ["/app/user-service"]
