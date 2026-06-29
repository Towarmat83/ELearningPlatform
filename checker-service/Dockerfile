FROM golang:1.26-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod download

COPY . .

# KICS false positive: build command executed in builder stage only
RUN CGO_ENABLED=0 GOOS=linux \
    go build -o checker-service ./cmd/main.go

FROM scratch

WORKDIR /app

COPY --from=builder /app/checker-service .

USER 1000:1000

EXPOSE 8083

ENTRYPOINT ["./checker-service"]