.PHONY: dev-db test lint tidy

dev-db:
	docker compose up -d postgres mailpit

test:
	go test ./... -count=1

lint:
	go vet ./...
	@test -z "$$(gofmt -l .)" || (echo "gofmt needed on:"; gofmt -l .; exit 1)

tidy:
	go mod tidy

.PHONY: web web-test build up down e2e

web:
	cd web && pnpm install --frozen-lockfile && pnpm build

web-test:
	cd web && pnpm install --frozen-lockfile && pnpm typecheck && pnpm test

build: web
	go build -o bin/openforms ./cmd/openforms

up:
	docker compose --profile app up -d --build

down:
	docker compose --profile app down -v

e2e: up
	cd e2e && pnpm install --frozen-lockfile && pnpm exec playwright install chromium && pnpm test
