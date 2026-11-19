# syntax=docker/dockerfile:1.7

FROM node:22-alpine AS web-build
WORKDIR /src
COPY package.json package-lock.json ./
RUN npm ci
COPY index.html tsconfig.json tsconfig.app.json vite.config.ts ./
COPY src ./src
RUN npm run build

FROM golang:1.25-alpine AS go-build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/codeflow-server ./cmd/server \
    && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/codeflow-container ./cmd/container

FROM ghcr.io/pocket-id/pocket-id:v2.19.3 AS pocket-id

FROM caddy:2.11.9-alpine
WORKDIR /app
COPY --from=go-build /out/codeflow-server /usr/local/bin/codeflow-server
COPY --from=go-build /out/codeflow-container /usr/local/bin/codeflow-container
COPY --from=web-build /src/dist /app/dist
COPY --from=pocket-id /app/pocket-id /usr/local/bin/pocket-id
COPY --from=pocket-id /sbin/su-exec /sbin/su-exec
COPY deploy/Caddyfile /etc/caddy/Caddyfile

ENV CODEFLOW_STATE_ROOT=/data \
    CODEFLOW_REPOSITORIES_PATH=/repos \
    CODEFLOW_CADDYFILE=/etc/caddy/Caddyfile

RUN mkdir -p /data/codeflow /data/pocket-id /data/caddy-data /data/caddy-config /data/home /repos \
    && chown -R 1000:1000 /app /data/codeflow /data/pocket-id /data/caddy-data /data/caddy-config /data/home

VOLUME ["/data"]
EXPOSE 8080 8081 8443
STOPSIGNAL SIGTERM
HEALTHCHECK --interval=30s --timeout=5s --start-period=30s --retries=3 \
  CMD wget -qO- http://127.0.0.1:8787/api/health >/dev/null && /usr/local/bin/pocket-id healthcheck

ENTRYPOINT ["/usr/local/bin/codeflow-container"]
