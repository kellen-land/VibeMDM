VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)
export PATH := /config/.local/node/bin:$(PATH)

.PHONY: all build web agent agent-macos sim server test vet docker up down logs dev-db clean

all: web build agent sim

## Build the server binary (embeds whatever is in internal/webui/dist)
build server:
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/vibemdm ./cmd/vibemdm

## Build the web UI into internal/webui/dist
web:
	cd web && npm ci --no-audit --no-fund && npm run build

## Cross-compile the macOS agent for Apple silicon and Intel
agent:
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/agents/vibemdm-agent-darwin-arm64 ./cmd/vibemdm-agent
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/agents/vibemdm-agent-darwin-amd64 ./cmd/vibemdm-agent

## Build the agent on a Mac with cgo, which adds macOS unified logging (os_log).
## Needs Xcode or the Command Line Tools; cannot run on Linux.
agent-macos:
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=1 CC="clang -arch arm64" go build -trimpath -ldflags "$(LDFLAGS)" -o bin/agents/vibemdm-agent-darwin-arm64 ./cmd/vibemdm-agent
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=1 CC="clang -arch x86_64" go build -trimpath -ldflags "$(LDFLAGS)" -o bin/agents/vibemdm-agent-darwin-amd64 ./cmd/vibemdm-agent

## Build the device simulator
sim:
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/vibemdm-sim ./cmd/vibemdm-sim

test:
	go test ./...

vet:
	go vet ./...
	cd web && npm run typecheck

## Docker image
docker:
	docker build -f deploy/Dockerfile --build-arg VERSION=$(VERSION) -t vibemdm-local:$(VERSION) -t vibemdm-local:latest .

up:
	cd deploy && docker compose up -d --build

down:
	cd deploy && docker compose down

logs:
	cd deploy && docker compose logs -f vibemdm

## Local Postgres for development (no TLS, port 5432)
dev-db:
	docker run -d --name vibemdm-dev-db -e POSTGRES_PASSWORD=vibemdm -e POSTGRES_USER=vibemdm -e POSTGRES_DB=vibemdm -p 127.0.0.1:5432:5432 postgres:17-alpine

clean:
	rm -rf bin internal/webui/dist/assets internal/webui/dist/index.html
