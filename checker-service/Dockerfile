FROM golang:1.26-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux  \
  go build -o checker-service ./cmd/main.go

FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata && \
    addgroup -S app && \
    adduser -S app -G app

WORKDIR /app

COPY --from=builder /app/checker-service .

USER app

EXPOSE 8083

HEALTHCHECK --interval=30s --timeout=5s \
    CMD pidof checker-service || exit 1
    
ENTRYPOINT ["./checker-service"]
