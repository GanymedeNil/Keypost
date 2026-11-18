.PHONY: dev-backend dev-frontend test test-pg build up down fmt lint

GOFLAGS ?=
PG_URL  ?= postgres://postgres:pw@localhost:55433/fulfillment?sslmode=disable

dev-backend:            ## run the Go API on :8080 with SQLite in ./backend/data
	cd backend && PUBLIC_URL=http://localhost:3000 go run ./cmd/server

dev-frontend:           ## run the Next.js app on :3000, proxying to :8080
	cd frontend && pnpm dev

test:                   ## backend tests (SQLite in memory) + race detector
	cd backend && go test ./... -race -count=1

test-pg:                ## backend tests against PostgreSQL (starts a throwaway container)
	-docker rm -f df-pg >/dev/null 2>&1
	docker run -d --name df-pg -e POSTGRES_PASSWORD=pw -e POSTGRES_DB=fulfillment -p 55433:5432 postgres:16-alpine
	until docker exec df-pg pg_isready -U postgres -d fulfillment >/dev/null 2>&1; do sleep 1; done
	# -p 1: the packages share one database and each test run recreates the schema
	cd backend && TEST_DATABASE_URL='$(PG_URL)' go test ./... -count=1 -p 1; rc=$$?; docker rm -f df-pg >/dev/null; exit $$rc

build:
	cd backend && go build -o bin/server ./cmd/server && go build -o bin/dfctl ./cmd/dfctl
	cd frontend && pnpm build

fmt:
	cd backend && gofmt -w .

lint:
	cd backend && go vet ./...
	cd frontend && pnpm lint && pnpm typecheck

up:
	docker compose up -d --build

down:
	docker compose down
