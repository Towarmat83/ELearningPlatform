# Stage 1: Build
FROM node:22-alpine AS builder

WORKDIR /app

COPY package.json package-lock.json* ./
RUN npm ci

COPY . .
RUN npm run build

# Stage 2: Runtime
FROM node:22-alpine

WORKDIR /app

COPY --from=builder /app/build ./build
COPY --from=builder /app/package.json .

ENV NODE_ENV=production
ENV PORT=3000
ENV FRONTEND_CONFIG=/etc/frontend/config.env

COPY docker-entrypoint.sh /docker-entrypoint.sh
RUN chmod +x /docker-entrypoint.sh

EXPOSE 3000

HEALTHCHECK --interval=30s --timeout=10s --retries=3 \
    CMD wget -qO- http://localhost:3000/ || exit 1

ENTRYPOINT ["/docker-entrypoint.sh"]
