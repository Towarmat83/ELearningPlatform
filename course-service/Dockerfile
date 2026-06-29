# Stage 1: Build
FROM docker.io/library/golang:1.26-alpine AS builder

RUN apk add --no-cache ca-certificates git

WORKDIR /app

# Cache module downloads
COPY go.mod go.sum ./
RUN go mod download

# Build binary
COPY . .
RUN CGO_ENABLED=0 GOOS=linux \
  go build -ldflags="-s -w" -o course-service .

# Stage 2: Runtime
FROM docker.io/library/alpine:3.21

RUN apk add --no-cache ca-certificates curl  && \
    addgroup -S appgroup && \
    adduser -S appuser -G appgroup

WORKDIR /app

COPY --from=builder /app/course-service /app/course-service

# No ENV config defaults here: env vars override the mounted ConfigMap file
# (precedence: code defaults → CONFIG_PATH file → env). Defaults live in
# internal/config/config.go.

USER appuser    

EXPOSE 8082

HEALTHCHECK --interval=30s --timeout=10s --start-period=10s --retries=3 \
    CMD curl -f http://localhost:8082/health || exit 1

ENTRYPOINT ["/app/course-service"]
