FROM golang:1.26-alpine AS builder

RUN apk add --no-cache ca-certificates git

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o ldap-service .

FROM alpine:3.21

RUN apk add --no-cache ca-certificates curl

WORKDIR /app

COPY --from=builder /app/ldap-service /app/ldap-service

ENV PORT=8083 \
    DATABASE_URL="postgres://elearning:elearning@localhost:5432/elearning" \
    USER_SERVICE_URL="http://user-service:8081" \
    CORS_ORIGINS="http://localhost:3000"

EXPOSE 8083

HEALTHCHECK --interval=30s --timeout=10s --start-period=10s --retries=3 \
    CMD curl -f http://localhost:8083/health || exit 1

ENTRYPOINT ["/app/ldap-service"]
