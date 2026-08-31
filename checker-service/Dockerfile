# Stage 1: Build
#
# The build context is the repository root, not this directory: all Go
# services share the root go.mod and the packages under internal/, so a
# per-service context can no longer see everything the build needs.
# Build with:  docker build -f checker-service/Dockerfile .
FROM docker.io/library/golang:1.26.7-alpine AS builder

WORKDIR /src

# Cache module downloads. The go.mod is now shared, so any dependency change
# invalidates this layer for every service; the cache mounts below keep that
# from costing a full re-download and re-compile each time.
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux \
    go build -ldflags="-s -w" -o /out/checker-service ./checker-service/cmd

# Stage 2: Runtime
FROM scratch

WORKDIR /app

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /out/checker-service /app/checker-service

# No ENV config defaults here: env vars override the mounted ConfigMap file
# (precedence: code defaults → CONFIG_PATH file → env). Defaults live in
# checker-service/internal/config/config.go.

USER 1000:1000

EXPOSE 8083

HEALTHCHECK NONE

ENTRYPOINT ["/app/checker-service"]
