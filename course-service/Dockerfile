# Stage 1: Build
FROM docker.io/library/golang:1.26.7-alpine AS builder

WORKDIR /app

# Cache module downloads
COPY go.mod go.sum ./
RUN go mod download

# Build binary
COPY . .
RUN CGO_ENABLED=0 GOOS=linux \
    go build -ldflags="-s -w" -o course-service .

# Stage 2: Runtime
FROM scratch

WORKDIR /app

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /app/course-service /app/course-service

# No ENV config defaults here: env vars override the mounted ConfigMap file
# (precedence: code defaults → CONFIG_PATH file → env). Defaults live in
# internal/config/config.go.

USER 1000:1000

EXPOSE 8082

HEALTHCHECK NONE

ENTRYPOINT ["/app/course-service"]
