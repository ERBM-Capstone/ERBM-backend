.PHONY: dev build run \
        migrate-up migrate-down migrate-create \
        tidy vet clean swagger \
        docker-up docker-down docker-build docker-logs docker-restart docker-shell

# ── Environment ───────────────────────────────────────────────────────────────
ifneq (,$(wildcard ./.env))
    include .env
    export
endif

APP_NAME   := ERBM-backend
IMAGE_NAME := $(APP_NAME)
IMAGE_TAG  ?= latest

# ── Local development ─────────────────────────────────────────────────────────

## dev: generate Swagger docs then run the server locally
dev: swagger
	go run ./cmd/server

## build: compile the server binary into bin/
build: swagger
	go build -o bin/server ./cmd/server

## run: build then execute the binary
run: build
	./bin/server

# ── Swagger ───────────────────────────────────────────────────────────────────

## swagger: regenerate Swagger docs from handler annotations
swagger:
	go run github.com/swaggo/swag/cmd/swag init -g cmd/server/main.go -o docs

# ── Database migrations ───────────────────────────────────────────────────────

## migrate-up: apply all pending migrations
migrate-up:
	go run ./cmd/migrate up

## migrate-down: roll back the last applied migration
migrate-down:
	go run ./cmd/migrate down

## migrate-create: scaffold paired sequential up/down migration files (prompts for name)
migrate-create:
	@read -p "Migration name: " name; \
	last=$$(find migrations -maxdepth 1 -type f -name '[0-9][0-9][0-9][0-9]_*.up.sql' -exec basename {} .up.sql \; | sed 's/_.*//' | sort -n | tail -n 1); \
	next=$$(awk -v n="$${last:-0}" 'BEGIN { printf "%04d", n + 1 }'); \
	base="migrations/$${next}_$${name}"; \
	cp migrations/.template.up.sql "$${base}.up.sql"; \
	cp migrations/.template.down.sql "$${base}.down.sql"; \
	echo "Created $${base}.up.sql"; \
	echo "Created $${base}.down.sql"

# ── Docker ────────────────────────────────────────────────────────────────────

## docker-up: start all containers in the background
docker-up:
	docker compose up -d

## docker-down: stop and remove all containers (keeps volumes)
docker-down:
	docker compose down

## docker-build: build the app Docker image
docker-build:
	docker build -t $(IMAGE_NAME):$(IMAGE_TAG) .

## docker-logs: tail logs from all containers
docker-logs:
	docker compose logs -f

## docker-restart: restart all containers
docker-restart:
	docker compose restart
# ── Go tools ──────────────────────────────────────────────────────────────────

## tidy: tidy go.mod / go.sum
tidy:
	go mod tidy

## vet: run go vet on all packages
vet:
	go vet ./...

## test: run all tests with coverage
test:
	go test ./... -v -cover

# ── Clean ─────────────────────────────────────────────────────────────────────

## clean: remove compiled binaries
clean:
	rm -rf bin/

## help: list all targets with descriptions
help:
	@grep -E '^## ' Makefile | sed 's/## //'
