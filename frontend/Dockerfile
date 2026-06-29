# Stage 1: Build
FROM docker.io/library/node:22-alpine AS builder

WORKDIR /app

COPY package.json package-lock.json* ./

RUN npm ci

COPY . .

RUN npm run build

# Stage 2: Runtime
FROM docker.io/library/node:22-alpine

RUN addgroup -g 1000 -S appgroup && \
    adduser -u 1000 -S appuser -G appgroup

WORKDIR /app

COPY --from=builder /app/dist ./dist

COPY --from=builder /app/package.json \
                     /app/package-lock.json ./

RUN npm ci --omit=dev

COPY --chmod=755 docker-entrypoint.sh /docker-entrypoint.sh

RUN chown -R 1000:1000 /app /docker-entrypoint.sh

USER 1000:1000

ENV NODE_ENV=production

ENV PORT=3000

ENV HOST=0.0.0.0

ENV FRONTEND_CONFIG=/etc/frontend/config.env

EXPOSE 3000

HEALTHCHECK --interval=30s --timeout=10s --retries=3 \
    CMD wget -qO- http://localhost:3000/ || exit 1

ENTRYPOINT ["/docker-entrypoint.sh"]