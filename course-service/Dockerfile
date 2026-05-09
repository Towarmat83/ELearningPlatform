# Stage 1: Build
FROM golang:1.26-alpine AS builder

RUN apk add --no-cache ca-certificates git

WORKDIR /app

# Cache module downloads
COPY go.mod go.sum ./
RUN go mod download

# Build binary
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o course-service .

# Stage 2: Runtime
FROM alpine:3.21

RUN apk add --no-cache ca-certificates curl git

WORKDIR /app

COPY --from=builder /app/course-service /app/course-service

# ── All configuration via env vars (ConfigMap friendly) ──────────────────────

ENV PORT=8082 \
    CONFIG_PATH="" \
    JWT_SECRET="change-me-in-production-use-a-long-random-string" \
    JWT_EXPIRY_HOURS=24 \
    CORS_ORIGINS="http://localhost:3000,http://localhost:5173" \
    UPLOADS_DIR="./uploads" \
    KUBECONFIG="" \
    K8S_NAMESPACE="default" \
    USER_SERVICE_URL="http://user-service:8081" \
    GIT_TOKEN="" \
    GIT_CREDENTIALS_PATH="/etc/course-service/git-credentials.yaml"

EXPOSE 8082

HEALTHCHECK --interval=30s --timeout=10s --start-period=10s --retries=3 \
    CMD curl -f http://localhost:8082/health || exit 1

ENTRYPOINT ["/app/course-service"]
