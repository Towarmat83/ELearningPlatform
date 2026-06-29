# Stage 1: Build
FROM docker.io/library/node:22-alpine AS builder

WORKDIR /app

COPY package.json package-lock.json* ./
RUN npm ci

COPY . .
RUN npm run build


# Stage 2: Runtime
FROM docker.io/library/node:22-alpine

RUN addgroup -S appgroup && \
    adduser -S appuser -G appgroup

WORKDIR /app

COPY --from=builder /app/dist ./dist
COPY --from=builder /app/package.json \
                     /app/package-lock.json ./

RUN npm ci --omit=dev

COPY docker-entrypoint.sh /docker-entrypoint.sh
RUN chmod +x /docker-entrypoint.sh

RUN chown -R appuser:appgroup /app /docker-entrypoint.sh

USER appuser

ENV NODE_ENV=production
ENV PORT=3000
ENV HOST=0.0.0.0
ENV FRONTEND_CONFIG=/etc/frontend/config.env

EXPOSE 3000

HEALTHCHECK --interval=30s --timeout=10s --retries=3 \
    CMD wget -qO- http://localhost:3000/ || exit 1

ENTRYPOINT ["/docker-entrypoint.sh"]