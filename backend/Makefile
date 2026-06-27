# C5 backend — developer workflow. `make help` lists targets.
.DEFAULT_GOAL := help
SHELL := /bin/bash

# golang-migrate CLI run via `go run` with the postgres driver build tag, so no
# global install is required. Override MIGRATE / DB_DSN as needed.
MIGRATE     ?= go run -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate
MIGRATIONS  ?= db/migrations
DB_DSN      ?= $(C5_DB_DSN)
API_IMAGE   ?= c5-api:dev
WORKER_IMAGE?= c5-worker:dev

.PHONY: help
help: ## Show this help
	@grep -hE '^[a-zA-Z0-9_-]+:.*?## ' $(MAKEFILE_LIST) | sort | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'

# ---------------------------------------------------------------------------
# Code generation (spec-first). CI runs `make gen && git diff --exit-code`.
# ---------------------------------------------------------------------------
# sqlc runs via its official image by default (the source build pulls modernc
# sqlite, which fails to compile on recent macOS SDKs). Override with a native
# binary: `make gen-sqlc SQLC=sqlc`.
SQLC ?= docker run --rm -v "$(CURDIR):/src" -w /src sqlc/sqlc:1.27.0

.PHONY: gen gen-oapi gen-sqlc
gen: gen-oapi gen-sqlc ## Run all code generation (CI: make gen && git diff --exit-code)

gen-oapi: ## Generate Gin server stubs + types from api/openapi.yaml
	go tool oapi-codegen -config oapi-codegen.yaml api/openapi.yaml

gen-sqlc: ## Generate type-safe queries from db/queries (sqlc via docker)
	$(SQLC) generate -f sqlc.yaml

# ---------------------------------------------------------------------------
# Build / run / quality
# ---------------------------------------------------------------------------
.PHONY: tidy fmt vet lint build run test test-integration cover
tidy: ## go mod tidy
	go mod tidy

fmt: ## gofmt + goimports (via gofmt -s)
	gofmt -s -w $(shell find . -name '*.go' -not -path './internal/gen/*' -not -path './Android-SDK*')

vet: ## go vet
	go vet ./...

lint: ## golangci-lint (skips if not installed)
	@command -v golangci-lint >/dev/null 2>&1 && golangci-lint run ./... || echo "golangci-lint not installed; skipping (CI enforces it)"

build: ## Build all binaries
	go build ./...

run: ## Run c5-api locally (expects C5_* env)
	go run ./cmd/api

test: ## Unit tests (race + coverage)
	go test -race -cover ./...

test-integration: ## Integration tests (testcontainers PostGIS + Redis; needs Docker)
	go test -tags=integration -race ./...

cover: ## Unit coverage profile -> coverage.out
	go test -race -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out | tail -1

# ---------------------------------------------------------------------------
# Database migrations (golang-migrate). Production runs these as a serial Job.
# ---------------------------------------------------------------------------
.PHONY: migrate-up migrate-down migrate-create
migrate-up: ## Apply all up migrations (DB_DSN or C5_DB_DSN)
	$(MIGRATE) -path $(MIGRATIONS) -database "$(DB_DSN)" up

migrate-down: ## Roll back one migration
	$(MIGRATE) -path $(MIGRATIONS) -database "$(DB_DSN)" down 1

migrate-create: ## Create a new migration: make migrate-create NAME=add_x
	$(MIGRATE) create -ext sql -dir $(MIGRATIONS) -seq $(NAME)

# ---------------------------------------------------------------------------
# Containers
# ---------------------------------------------------------------------------
.PHONY: compose-up compose-down docker-build
compose-up: ## Start local PostGIS + Redis
	docker compose -f deploy/docker-compose.dev.yaml up -d

compose-down: ## Stop local PostGIS + Redis
	docker compose -f deploy/docker-compose.dev.yaml down

docker-build: ## Build the c5-api image
	docker build -f deploy/Dockerfile.api -t $(API_IMAGE) .
