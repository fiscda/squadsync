# Local development for Squad Sync. Run `make help` for the targets.
#
# Configuration comes from the root .env file (copy .env.example). Every
# variable in it is exported to both servers and to the migration commands.

-include .env
export

DATABASE_PATH ?= ./data/squadsync.db
BACKEND_PORT ?= 8081
# Both servers run in their own directory, so pass them an absolute path.
override DATABASE_PATH := $(abspath $(DATABASE_PATH))
export DATABASE_PATH

.PHONY: help dev migrate migrate-be migrate-fe install test lint check-env

help:
	@echo "make install  install frontend dependencies"
	@echo "make migrate  migrate the SQLite file (Go tables, then Better Auth tables)"
	@echo "make dev      migrate, then run Go (127.0.0.1:$(BACKEND_PORT)) and Next.js (localhost:3000)"
	@echo "make test     run the Go tests"
	@echo "make lint     lint and type-check the frontend"

install:
	cd squadsync-fe && bun install

check-env:
	@test -f .env || { echo "Missing .env: copy .env.example to .env and fill it in (see README.md)."; exit 1; }

migrate: check-env migrate-be migrate-fe

migrate-be:
	cd squadsync-be && go run ./cmd/squadsync migrate

migrate-fe:
	cd squadsync-fe && bun run auth:migrate

# Runs both servers; Ctrl+C stops both.
dev: migrate
	@trap 'trap - INT TERM EXIT; kill 0' INT TERM EXIT; \
	(cd squadsync-be && go run ./cmd/squadsync) & \
	(cd squadsync-fe && bun run dev) & \
	wait

test:
	cd squadsync-be && go test ./...

lint:
	cd squadsync-fe && bun run lint && bunx next typegen && bunx tsc --noEmit
