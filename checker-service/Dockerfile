# Stage 1: Build
FROM docker.io/library/golang:1.26-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux \
    go build -o checker-service ./cmd/main.go

# Stage 2: Runtime
FROM scratch

WORKDIR /app

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /app/checker-service .

USER 1000:1000

EXPOSE 8083

HEALTHCHECK NONE

ENTRYPOINT ["./checker-service"]
