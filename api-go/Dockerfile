# Stage 1: Build
FROM golang:1.25-alpine AS builder

RUN apk add --no-cache ca-certificates git

WORKDIR /app

# Cache module downloads
COPY go.mod go.sum ./
RUN go mod download

# Build binary
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o elearning-api .

# Stage 2: Runtime
FROM alpine:3.21

RUN apk add --no-cache ca-certificates curl git

WORKDIR /app

COPY --from=builder /app/elearning-api /app/elearning-api

ENV PORT=8080
EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=10s --start-period=10s --retries=3 \
    CMD curl -f http://localhost:8080/health || exit 1

ENTRYPOINT ["/app/elearning-api"]
