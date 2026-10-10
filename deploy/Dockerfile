# syntax=docker/dockerfile:1.7
# ---- web UI ----
FROM node:24-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json* ./
RUN npm ci --no-audit --no-fund
COPY web/ ./
RUN npm run build

# ---- Go build ----
FROM golang:1.27-alpine AS build
ARG VERSION=dev
WORKDIR /src
RUN apk add --no-cache git
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
COPY --from=web /src/internal/webui/dist ./internal/webui/dist
ENV CGO_ENABLED=0
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/vibemdm ./cmd/vibemdm && \
    go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/vibemdm-sim ./cmd/vibemdm-sim && \
    GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/agents/vibemdm-agent-darwin-arm64 ./cmd/vibemdm-agent && \
    GOOS=darwin GOARCH=amd64 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/agents/vibemdm-agent-darwin-amd64 ./cmd/vibemdm-agent

# ---- runtime ----
FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata && \
    addgroup -S vibemdm && adduser -S -G vibemdm -h /app vibemdm && \
    mkdir -p /data && chown vibemdm:vibemdm /data
COPY --from=build /out/vibemdm /app/vibemdm
COPY --from=build /out/vibemdm-sim /app/vibemdm-sim
COPY --from=build /out/agents /app/agents
ENV VIBEMDM_LISTEN=:8080 VIBEMDM_DATA_DIR=/data VIBEMDM_AGENT_DIR=/app/agents
USER vibemdm
WORKDIR /app
VOLUME ["/data"]
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=20s CMD wget -qO- http://127.0.0.1:8080/healthz || exit 1
ENTRYPOINT ["/app/vibemdm"]
